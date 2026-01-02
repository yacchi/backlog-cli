package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/auth"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/debug"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

// 認証方式
const (
	authMethodOAuth  = "OAuth 2.0"
	authMethodAPIKey = "API Key"
)

var (
	loginDomain            string
	loginSpace             string
	loginNoBrowser         bool
	loginCallbackPort      int
	loginTimeout           int
	loginReuse             bool
	loginWeb               bool
	loginForceBundleUpdate bool
)

func init() {
	loginCmd.Flags().StringVar(&loginDomain, "domain", "", "Backlog domain (backlog.jp or backlog.com)")
	loginCmd.Flags().StringVar(&loginSpace, "space", "", "Backlog space name")
	loginCmd.Flags().BoolVar(&loginNoBrowser, "no-browser", false, "Don't open browser, just print URL")
	loginCmd.Flags().IntVar(&loginCallbackPort, "callback-port", 0, "Fixed port for callback server")
	loginCmd.Flags().IntVar(&loginTimeout, "timeout", 0, "Timeout in seconds (default: 120)")
	loginCmd.Flags().BoolVarP(&loginReuse, "reuse", "r", false, "Reuse previous login settings (method, space, domain) without prompts")
	loginCmd.Flags().BoolVar(&loginWeb, "web", false, "Use web-based authentication (all prompts in browser)")
	loginCmd.Flags().BoolVar(&loginForceBundleUpdate, "force-bundle-update", false, "Force bundle update check (debug)")
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to Backlog",
	Long: `Authenticate with Backlog using OAuth 2.0 or API Key.

If a relay server is configured, you can choose between OAuth 2.0 and
API Key authentication. If no relay server is configured, only API Key
authentication is available.

For OAuth 2.0, this command opens a browser window for authentication.
If the browser cannot be opened, a URL will be displayed for manual access.

For API Key authentication, you will be prompted to enter your API Key.
You can obtain your API Key from your Backlog personal settings page.

On first login, you will be prompted to select a domain and enter your
space name. These settings will be saved to your profile.

Use --reuse (-r) flag to skip prompts and reuse previous login settings
(authentication method, space, and domain).

Use --web flag to perform all authentication steps in the browser,
which is useful for automation or when terminal input is not available.`,
	RunE: runLogin,
}

func runLogin(cmd *cobra.Command, args []string) error {
	// 設定読み込み
	cfg, err := config.Load(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// --web オプションが指定された場合はWebベースの認証フローを使用
	if loginWeb {
		return runWebLogin(cmd.Context(), cfg)
	}

	// 中継サーバーの確認
	profile := cfg.CurrentProfile()

	// 認証方式の決定
	var authMethod string

	// --reuse オプションが指定された場合
	if loginReuse {
		// 既存のクレデンシャルを取得
		resolved := cfg.Resolved()
		cred := resolved.GetActiveCredential()
		if cred == nil {
			return fmt.Errorf("no previous login found. Please run 'backlog auth login' without --reuse flag first")
		}

		// 以前の認証方式を使用
		switch cred.GetAuthType() {
		case config.AuthTypeOAuth:
			// OAuthの場合、relay_serverの設定がなくても--reuseならスキップできる
			if profile != nil && profile.RelayServer != "" {
				authMethod = authMethodOAuth
			} else {
				return fmt.Errorf("OAuth authentication requires a relay server, but none is configured")
			}
		case config.AuthTypeAPIKey:
			authMethod = authMethodAPIKey
		default:
			return fmt.Errorf("unknown authentication type in previous credentials")
		}

		if authMethod == authMethodOAuth && profile != nil && profile.Space != "" && profile.Domain != "" {
			fmt.Printf("Reusing previous login settings: %s with %s.%s\n", authMethod, profile.Space, profile.Domain)
		}
	} else {
		// 通常の認証方式選択（OAuth は常に選択可能）
		authMethod, err = ui.Select("Select authentication method:", []string{authMethodOAuth, authMethodAPIKey})
		if err != nil {
			return err
		}
	}

	// 認証方式に応じて処理を分岐
	ctx := cmd.Context()
	if authMethod == authMethodAPIKey {
		return runAPIKeyLogin(ctx, cfg)
	}
	return runOAuthLogin(ctx, cfg)
}

// runAPIKeyLogin はAPI Key認証を実行する
func runAPIKeyLogin(ctx context.Context, cfg *config.Store) error {
	// オプションのマージ
	opts := mergeLoginOptions(cfg)

	// 設定変更の確認フロー
	configChanged := false
	if loginSpace != "" {
		opts.space = loginSpace
		configChanged = true
	}
	if loginDomain != "" {
		opts.domain = loginDomain
		configChanged = true
	}

	// --reuse オプションが指定されていない場合のみ確認ダイアログを表示
	if !opts.reuse {
		// 既存設定があり、コマンドライン引数で指定されていない場合
		if !configChanged && opts.space != "" && opts.domain != "" {
			fmt.Printf("Current settings: %s.%s\n", opts.space, opts.domain)
			changeSettings, err := ui.Confirm("Change space/domain settings?", false)
			if err != nil {
				return err
			}
			if changeSettings {
				configChanged = true
				opts.space = ""
				opts.domain = ""
			}
		}
	}

	// ドメイン選択（未設定または変更モードの場合）
	supportedDomains := []string{"backlog.jp", "backlog.com"}
	if opts.domain == "" {
		var err error
		opts.domain, err = ui.Select("Select Backlog domain:", supportedDomains)
		if err != nil {
			return err
		}
		configChanged = true
	} else if !slices.Contains(supportedDomains, opts.domain) {
		return fmt.Errorf("domain '%s' is not supported\nSupported: %v", opts.domain, supportedDomains)
	}

	// スペース入力（未設定または変更モードの場合）
	if opts.space == "" {
		var err error
		opts.space, err = ui.Input("Enter space name:", "")
		if err != nil {
			return err
		}
		if opts.space == "" {
			return fmt.Errorf("space name is required")
		}
		configChanged = true
	}

	// API Key入力
	fmt.Println()
	fmt.Printf("Authenticating with %s.%s using API Key...\n", opts.space, opts.domain)
	fmt.Println()
	fmt.Println("You can obtain your API Key from:")
	fmt.Printf("  https://%s.%s/EditApiSettings.action\n", opts.space, opts.domain)
	fmt.Println()

	apiKey, err := ui.Password("Enter API Key:")
	if err != nil {
		return err
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return fmt.Errorf("API Key is required")
	}

	// API Keyの検証（ユーザー情報取得）
	fmt.Println("Verifying API Key...")
	apiClient := api.NewClient(opts.space, opts.domain, "", api.WithAPIKey(apiKey))
	user, err := apiClient.GetCurrentUser(ctx)
	if err != nil {
		return fmt.Errorf("API Key verification failed: %w", err)
	}

	fmt.Printf("Authenticated as %s", user.Name.Value)
	if user.MailAddress.Value != "" {
		fmt.Printf(" (%s)", user.MailAddress.Value)
	}
	fmt.Println()

	// 認証情報保存（プロファイルに紐づける）
	profileName := cfg.GetActiveProfile()
	cred := &config.Credential{
		AuthType: config.AuthTypeAPIKey,
		APIKey:   apiKey,
		UserID:   user.UserId.Value,
		UserName: user.Name.Value,
	}
	_ = cfg.SetCredential(profileName, cred)

	// 設定が変更された場合はプロファイルに保存
	if configChanged {
		_ = cfg.SetProfileValue(config.LayerUser, profileName, "space", opts.space)
		_ = cfg.SetProfileValue(config.LayerUser, profileName, "domain", opts.domain)

		if err := cfg.Reload(ctx); err != nil {
			return fmt.Errorf("failed to reload config: %w", err)
		}
	}

	// 設定を保存
	if err := cfg.Save(ctx); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Logged in to %s.%s\n", opts.space, opts.domain)
	return nil
}

// runOAuthLogin はOAuth認証を実行する
// ブラウザベースの設定UIを使用する新フロー
func runOAuthLogin(ctx context.Context, cfg *config.Store) error {
	debug.Log("starting OAuth login")

	// オプションのマージ
	opts := mergeLoginOptions(cfg)
	debug.Log("login options merged", "callback_port", opts.callbackPort, "timeout", opts.timeout, "reuse", opts.reuse)

	profile := cfg.CurrentProfile()
	if profile != nil && profile.Space != "" && profile.Domain != "" && profile.RelayServer != "" {
		if err := verifyRelayInfoIfTrusted(ctx, cfg, profile.RelayServer, profile.Space, profile.Domain, loginForceBundleUpdate); err != nil {
			return err
		}
	}

	// 1. state 生成
	state, err := auth.GenerateState()
	if err != nil {
		return fmt.Errorf("failed to generate state: %w", err)
	}
	debug.Log("state generated", "state_length", len(state))

	// 2. コールバックサーバー起動（設定UIハンドラー付き）
	debug.Log("creating callback server", "requested_port", opts.callbackPort)
	callbackServer, err := auth.NewCallbackServer(auth.CallbackServerOptions{
		Port:              opts.callbackPort,
		State:             state,
		ConfigStore:       cfg,
		Reuse:             opts.reuse,
		ForceBundleUpdate: loginForceBundleUpdate,
		Ctx:               ctx,
	})
	if err != nil {
		return fmt.Errorf("failed to start callback server: %w", err)
	}
	debug.Log("callback server ready", "actual_port", callbackServer.Port())

	go func() {
		if err := callbackServer.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			debug.Log("callback server error", "error", err)
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := callbackServer.Shutdown(shutdownCtx); err != nil {
			debug.Log("callback server shutdown error", "error", err)
		}
	}()

	// 3. ローカルサーバーの /auth/start を開く
	localAuthURL := fmt.Sprintf("http://localhost:%d/auth/start", callbackServer.Port())

	fmt.Println()
	fmt.Println("Open this URL in your browser to log in:")
	fmt.Println()
	fmt.Printf("  %s\n", localAuthURL)
	fmt.Println()
	fmt.Println("Waiting for authentication... (press Ctrl+C to cancel)")

	if !opts.noBrowser {
		if err := browser.OpenURL(localAuthURL); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not open browser: %v\n", err)
		}
	}

	// 4. コールバック待機（タイムアウトなし - ロングポーリングベースのフローに対応）
	debug.Log("waiting for callback (no timeout)")
	result := callbackServer.Wait()

	if result.Error != nil {
		return fmt.Errorf("authentication failed: %w", result.Error)
	}

	// 5. 設定を再読み込み（ブラウザで設定が保存された後）
	if err := cfg.Reload(ctx); err != nil {
		return fmt.Errorf("failed to reload config: %w", err)
	}

	// 設定から space/domain/relayServer を取得
	profile = cfg.CurrentProfile()
	if profile == nil || profile.Space == "" || profile.Domain == "" || profile.RelayServer == "" {
		return fmt.Errorf("configuration incomplete after authentication")
	}
	space := profile.Space
	domain := profile.Domain
	currentRelayServer := profile.RelayServer

	// 6. トークン交換
	debug.Log("exchanging authorization code", "code_length", len(result.Code))
	fmt.Println("Exchanging authorization code...")
	client := auth.NewClient(currentRelayServer)
	tokenResp, err := client.ExchangeToken(auth.TokenRequest{
		GrantType: "authorization_code",
		Code:      result.Code,
		Domain:    domain,
		Space:     space,
		State:     state, // CLI が生成した state を渡す
	})
	if err != nil {
		return fmt.Errorf("failed to exchange token: %w", err)
	}
	debug.Log("token exchange successful", "expires_in", tokenResp.ExpiresIn)

	// 7. ユーザー情報取得
	apiClient := api.NewClient(space, domain, tokenResp.AccessToken)
	user, err := apiClient.GetCurrentUser(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not fetch user info: %v\n", err)
	} else {
		fmt.Printf("Authenticated as %s", user.Name.Value)
		if user.MailAddress.Value != "" {
			fmt.Printf(" (%s)", user.MailAddress.Value)
		}
		fmt.Println()
	}

	// 8. 認証情報保存（プロファイルに紐づける）
	profileName := cfg.GetActiveProfile()
	cred := &config.Credential{
		AuthType:     config.AuthTypeOAuth,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}
	if user != nil {
		cred.UserID = user.UserId.Value
		cred.UserName = user.Name.Value
	}
	_ = cfg.SetCredential(profileName, cred)

	// 設定を保存
	if err := cfg.Save(ctx); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Logged in to %s.%s\n", space, domain)

	return nil
}

type loginOptions struct {
	domain       string
	space        string
	noBrowser    bool
	callbackPort int
	timeout      int
	reuse        bool
	web          bool
}

func mergeLoginOptions(cfg *config.Store) loginOptions {
	opts := loginOptions{
		domain:       loginDomain,
		space:        loginSpace,
		noBrowser:    loginNoBrowser,
		callbackPort: loginCallbackPort,
		timeout:      loginTimeout,
		reuse:        loginReuse,
		web:          loginWeb,
	}

	// 設定ファイルからの補完
	profile := cfg.CurrentProfile()
	if profile != nil {
		if opts.domain == "" {
			opts.domain = profile.Domain
		}
		if opts.space == "" {
			opts.space = profile.Space
		}
		if opts.callbackPort == 0 {
			opts.callbackPort = profile.AuthCallbackPort
		}
		if opts.timeout == 0 {
			opts.timeout = profile.AuthTimeout
		}
		if !opts.noBrowser {
			opts.noBrowser = profile.AuthNoBrowser
		}
	}

	// デフォルト値
	if opts.timeout == 0 {
		opts.timeout = 120
	}

	return opts
}

// runWebLogin はWebベースの認証フローを実行する
// 認証方式の選択もブラウザで行い、端末での対話を不要にする
func runWebLogin(ctx context.Context, cfg *config.Store) error {
	debug.Log("starting web-based login")

	// オプションのマージ
	opts := mergeLoginOptions(cfg)
	debug.Log("login options merged", "callback_port", opts.callbackPort)

	profile := cfg.CurrentProfile()
	if profile != nil && profile.Space != "" && profile.Domain != "" && profile.RelayServer != "" {
		if err := verifyRelayInfoIfTrusted(ctx, cfg, profile.RelayServer, profile.Space, profile.Domain, loginForceBundleUpdate); err != nil {
			return err
		}
	}

	// 1. state 生成
	state, err := auth.GenerateState()
	if err != nil {
		return fmt.Errorf("failed to generate state: %w", err)
	}
	debug.Log("state generated", "state_length", len(state))

	// 2. コールバックサーバー起動（設定UIハンドラー付き）
	debug.Log("creating callback server", "requested_port", opts.callbackPort)
	callbackServer, err := auth.NewCallbackServer(auth.CallbackServerOptions{
		Port:              opts.callbackPort,
		State:             state,
		ConfigStore:       cfg,
		Reuse:             false, // --web モードでは常にブラウザで設定
		ForceBundleUpdate: loginForceBundleUpdate,
		Ctx:               ctx,
	})
	if err != nil {
		return fmt.Errorf("failed to start callback server: %w", err)
	}
	debug.Log("callback server ready", "actual_port", callbackServer.Port())

	go func() {
		if err := callbackServer.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			debug.Log("callback server error", "error", err)
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := callbackServer.Shutdown(shutdownCtx); err != nil {
			debug.Log("callback server shutdown error", "error", err)
		}
	}()

	// 3. ローカルサーバーの /auth/method を開く（認証方式選択画面）
	localAuthURL := fmt.Sprintf("http://localhost:%d/auth/method", callbackServer.Port())

	fmt.Println()
	fmt.Println("Open this URL in your browser to log in:")
	fmt.Println()
	fmt.Printf("  %s\n", localAuthURL)
	fmt.Println()
	fmt.Println("Waiting for authentication... (press Ctrl+C to cancel)")

	if !opts.noBrowser {
		if err := browser.OpenURL(localAuthURL); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not open browser: %v\n", err)
		}
	}

	// 4. コールバック待機
	debug.Log("waiting for callback (no timeout)")
	result := callbackServer.Wait()

	if result.Error != nil {
		return fmt.Errorf("authentication failed: %w", result.Error)
	}

	// 5. 設定を再読み込み
	if err := cfg.Reload(ctx); err != nil {
		return fmt.Errorf("failed to reload config: %w", err)
	}

	// API Key 認証の場合は既に完了している
	if result.Code == "api_key_auth" {
		profile := cfg.CurrentProfile()
		if profile != nil && profile.Space != "" && profile.Domain != "" {
			fmt.Printf("Logged in to %s.%s\n", profile.Space, profile.Domain)
		}
		return nil
	}

	// OAuth 認証の場合はトークン交換が必要
	profile = cfg.CurrentProfile()
	if profile == nil || profile.Space == "" || profile.Domain == "" || profile.RelayServer == "" {
		return fmt.Errorf("configuration incomplete after authentication")
	}
	space := profile.Space
	domain := profile.Domain
	currentRelayServer := profile.RelayServer

	// 6. トークン交換
	debug.Log("exchanging authorization code", "code_length", len(result.Code))
	fmt.Println("Exchanging authorization code...")
	client := auth.NewClient(currentRelayServer)
	tokenResp, err := client.ExchangeToken(auth.TokenRequest{
		GrantType: "authorization_code",
		Code:      result.Code,
		Domain:    domain,
		Space:     space,
		State:     state,
	})
	if err != nil {
		return fmt.Errorf("failed to exchange token: %w", err)
	}
	debug.Log("token exchange successful", "expires_in", tokenResp.ExpiresIn)

	// 7. ユーザー情報取得
	apiClient := api.NewClient(space, domain, tokenResp.AccessToken)
	user, err := apiClient.GetCurrentUser(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not fetch user info: %v\n", err)
	} else {
		fmt.Printf("Authenticated as %s", user.Name.Value)
		if user.MailAddress.Value != "" {
			fmt.Printf(" (%s)", user.MailAddress.Value)
		}
		fmt.Println()
	}

	// 8. 認証情報保存
	profileName := cfg.GetActiveProfile()
	cred := &config.Credential{
		AuthType:     config.AuthTypeOAuth,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}
	if user != nil {
		cred.UserID = user.UserId.Value
		cred.UserName = user.Name.Value
	}
	_ = cfg.SetCredential(profileName, cred)

	// 設定を保存
	if err := cfg.Save(ctx); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Logged in to %s.%s\n", space, domain)

	return nil
}

func verifyRelayInfoIfTrusted(ctx context.Context, cfg *config.Store, relayServer, space, domain string, forceUpdate bool) error {
	allowedDomain := space + "." + domain
	bundle := config.FindTrustedBundle(cfg, allowedDomain)
	if bundle == nil {
		debug.Log("trusted bundle not found; skipping relay info preflight", "allowed_domain", allowedDomain)
		return nil
	}

	cacheDir, err := cfg.GetCacheDir()
	if err != nil {
		debug.Log("failed to resolve cache dir", "error", err)
	}
	infoURL, err := config.BuildRelayInfoURL(relayServer, allowedDomain)
	if err != nil {
		return fmt.Errorf("failed to build relay info url: %w", err)
	}
	debug.Log("verifying relay info", "url", infoURL, "allowed_domain", allowedDomain)

	info, err := config.VerifyRelayInfo(ctx, relayServer, allowedDomain, bundle.BundleToken, bundle.RelayKeys, config.RelayInfoOptions{
		CacheDir:      cacheDir,
		CertsCacheTTL: bundle.CertsCacheTTL,
	})
	if err != nil {
		return fmt.Errorf("relay info verification failed: %w", err)
	}

	if err := config.CheckBundleUpdate(info, bundle, time.Now().UTC(), forceUpdate); err != nil {
		var updateErr *config.BundleUpdateRequiredError
		if errors.As(err, &updateErr) {
			bundleURL, urlErr := config.BuildRelayBundleURL(relayServer, allowedDomain)
			if urlErr != nil {
				return fmt.Errorf("failed to build relay bundle url: %w", urlErr)
			}
			debug.Log("fetching relay bundle", "url", bundleURL, "allowed_domain", allowedDomain)
			updated, updateErr := config.FetchAndImportRelayBundle(ctx, cfg, relayServer, allowedDomain, bundle.BundleToken, config.BundleFetchOptions{
				CacheDir:   cacheDir,
				NoDefaults: true,
			})
			if updateErr != nil {
				return fmt.Errorf("bundle update failed: %w", updateErr)
			}
			if err := cfg.Save(ctx); err != nil {
				return fmt.Errorf("failed to save updated bundle: %w", err)
			}
			if err := cfg.Reload(ctx); err != nil {
				return fmt.Errorf("failed to reload config after bundle update: %w", err)
			}
			fmt.Printf("Updated relay bundle for %s\n", updated.AllowedDomain)
		} else {
			return err
		}
	}

	debug.Log("relay info verified", "allowed_domain", allowedDomain)
	fmt.Printf("Verified trusted relay server for %s\n", allowedDomain)
	return nil
}
