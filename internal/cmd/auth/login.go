package auth

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/api"
	"github.com/yacchi/backlog-cli/internal/auth"
	"github.com/yacchi/backlog-cli/internal/config"
	"github.com/yacchi/backlog-cli/internal/debug"
	"github.com/yacchi/backlog-cli/internal/ui"
)

// 認証方式
const (
	authMethodOAuth  = "OAuth 2.0"
	authMethodAPIKey = "API Key"
)

var (
	loginDomain       string
	loginSpace        string
	loginNoBrowser    bool
	loginCallbackPort int
	loginTimeout      int
	loginReuse        bool
)

func init() {
	loginCmd.Flags().StringVar(&loginDomain, "domain", "", "Backlog domain (backlog.jp or backlog.com)")
	loginCmd.Flags().StringVar(&loginSpace, "space", "", "Backlog space name")
	loginCmd.Flags().BoolVar(&loginNoBrowser, "no-browser", false, "Don't open browser, just print URL")
	loginCmd.Flags().IntVar(&loginCallbackPort, "callback-port", 0, "Fixed port for callback server")
	loginCmd.Flags().IntVar(&loginTimeout, "timeout", 0, "Timeout in seconds (default: 120)")
	loginCmd.Flags().BoolVarP(&loginReuse, "reuse", "r", false, "Reuse previous login settings (method, space, domain) without prompts")
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
(authentication method, space, and domain).`,
	RunE: runLogin,
}

func runLogin(cmd *cobra.Command, args []string) error {
	// 設定読み込み
	cfg, err := config.Load(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// 中継サーバーの確認
	profile := cfg.CurrentProfile()
	relayServer := ""
	if profile != nil {
		relayServer = profile.RelayServer
	}

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

		// 既存のspace/domainを確認
		if profile == nil || profile.Space == "" || profile.Domain == "" {
			return fmt.Errorf("no previous space/domain settings found. Please run 'backlog auth login' without --reuse flag first")
		}

		// 以前の認証方式を使用
		switch cred.GetAuthType() {
		case config.AuthTypeOAuth:
			if relayServer == "" {
				return fmt.Errorf("OAuth authentication requires a relay server, but none is configured")
			}
			authMethod = authMethodOAuth
		case config.AuthTypeAPIKey:
			authMethod = authMethodAPIKey
		default:
			return fmt.Errorf("unknown authentication type in previous credentials")
		}

		fmt.Printf("Reusing previous login settings: %s with %s.%s\n", authMethod, profile.Space, profile.Domain)
	} else {
		// 通常の認証方式選択
		if relayServer == "" {
			// リレーサーバーが設定されていない場合はAPI Key認証のみ
			authMethod = authMethodAPIKey
		} else {
			// リレーサーバーが設定されている場合は選択
			authMethod, err = ui.Select("Select authentication method:", []string{authMethodOAuth, authMethodAPIKey})
			if err != nil {
				return err
			}
		}
	}

	// 認証方式に応じて処理を分岐
	if authMethod == authMethodAPIKey {
		return runAPIKeyLogin(cfg)
	}
	return runOAuthLogin(cfg, relayServer)
}

// runAPIKeyLogin はAPI Key認証を実行する
func runAPIKeyLogin(cfg *config.Store) error {
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
	user, err := apiClient.GetCurrentUser()
	if err != nil {
		return fmt.Errorf("API Key verification failed: %w", err)
	}

	fmt.Printf("Authenticated as %s", user.Name.Value)
	if user.MailAddress.Value != "" {
		fmt.Printf(" (%s)", user.MailAddress.Value)
	}
	fmt.Println()

	ctx := context.Background()

	// 認証情報保存（プロファイルに紐づける）
	profileName := cfg.GetActiveProfile()
	cred := &config.Credential{
		AuthType: config.AuthTypeAPIKey,
		APIKey:   apiKey,
		UserID:   user.UserId.Value,
		UserName: user.Name.Value,
	}
	cfg.SetCredential(profileName, cred)

	// 設定が変更された場合はプロファイルに保存
	if configChanged {
		cfg.SetProfileValue(config.LayerUser, profileName, "space", opts.space)
		cfg.SetProfileValue(config.LayerUser, profileName, "domain", opts.domain)

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
func runOAuthLogin(cfg *config.Store, relayServer string) error {
	debug.Log("starting OAuth login", "relay_server", relayServer)

	// オプションのマージ
	opts := mergeLoginOptions(cfg)
	debug.Log("login options merged", "domain", opts.domain, "space", opts.space, "callback_port", opts.callbackPort, "timeout", opts.timeout)

	// 認証クライアント作成
	client := auth.NewClient(relayServer)

	// 1. well-known からメタ情報取得
	fmt.Println("Fetching relay server information...")
	meta, err := client.FetchWellKnown()
	if err != nil {
		return fmt.Errorf("failed to connect to relay server: %w", err)
	}

	if len(meta.SupportedDomains) == 0 {
		return fmt.Errorf("relay server has no supported domains configured")
	}

	// 2. 設定変更の確認フロー
	// コマンドライン引数で明示的に指定されている場合はその値を使う
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
				// 設定変更モードなのでリセット
				opts.space = ""
				opts.domain = ""
			}
		}
	}

	// 3. ドメイン選択（未設定または変更モードの場合）
	if opts.domain == "" {
		if len(meta.SupportedDomains) == 1 {
			opts.domain = meta.SupportedDomains[0]
		} else {
			opts.domain, err = ui.Select("Select Backlog domain:", meta.SupportedDomains)
			if err != nil {
				return err
			}
		}
		configChanged = true
	} else if !slices.Contains(meta.SupportedDomains, opts.domain) {
		return fmt.Errorf("domain '%s' is not supported by this relay server\nSupported: %v", opts.domain, meta.SupportedDomains)
	}

	// 4. スペース入力（未設定または変更モードの場合）
	if opts.space == "" {
		opts.space, err = ui.Input("Enter space name:", "")
		if err != nil {
			return err
		}
		if opts.space == "" {
			return fmt.Errorf("space name is required")
		}
		configChanged = true
	}

	fmt.Printf("\nAuthenticating with %s.%s...\n", opts.space, opts.domain)

	// 4. コールバックサーバー起動
	debug.Log("creating callback server", "requested_port", opts.callbackPort)
	callbackServer, err := auth.NewCallbackServer(opts.callbackPort)
	if err != nil {
		return fmt.Errorf("failed to start callback server: %w", err)
	}
	debug.Log("callback server ready", "actual_port", callbackServer.Port())

	go callbackServer.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		callbackServer.Shutdown(ctx)
	}()

	// 5. 認証開始（auth_urlを取得、Cookieがクライアントに保存される）
	profile := cfg.CurrentProfile()
	project := ""
	if profile != nil {
		project = profile.Project
	}
	authResp, err := client.StartAuth(opts.domain, opts.space, callbackServer.Port(), project)
	if err != nil {
		return fmt.Errorf("failed to start authentication: %w", err)
	}

	// 6. URL表示 & ブラウザ起動
	fmt.Println()
	fmt.Println("If browser doesn't open automatically, visit this URL:")
	fmt.Println()
	fmt.Printf("  %s\n", authResp.AuthURL)
	fmt.Println()
	fmt.Printf("Waiting for authentication... (timeout: %ds)\n", opts.timeout)

	if !opts.noBrowser {
		if err := browser.OpenURL(authResp.AuthURL); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not open browser: %v\n", err)
		}
	}

	// 7. コールバック待機
	debug.Log("waiting for callback", "timeout_seconds", opts.timeout)
	resultCh := make(chan auth.CallbackResult, 1)
	go func() {
		resultCh <- callbackServer.Wait()
	}()

	var result auth.CallbackResult
	select {
	case result = <-resultCh:
		debug.Log("callback result received", "has_error", result.Error != nil)
	case <-time.After(time.Duration(opts.timeout) * time.Second):
		debug.Log("callback timeout")
		return fmt.Errorf("authentication timed out after %d seconds", opts.timeout)
	}

	if result.Error != nil {
		return fmt.Errorf("authentication failed: %w", result.Error)
	}

	// 8. トークン交換（stateでセッション追跡）
	debug.Log("exchanging authorization code", "code_length", len(result.Code))
	fmt.Println("Exchanging authorization code...")
	tokenResp, err := client.ExchangeToken(auth.TokenRequest{
		GrantType: "authorization_code",
		Code:      result.Code,
		Domain:    opts.domain,
		Space:     opts.space,
		State:     authResp.State,
	})
	if err != nil {
		return fmt.Errorf("failed to exchange token: %w", err)
	}
	debug.Log("token exchange successful", "expires_in", tokenResp.ExpiresIn)

	// 9. ユーザー情報取得
	apiClient := api.NewClient(opts.space, opts.domain, tokenResp.AccessToken)
	user, err := apiClient.GetCurrentUser()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not fetch user info: %v\n", err)
	} else {
		fmt.Printf("Authenticated as %s", user.Name.Value)
		if user.MailAddress.Value != "" {
			fmt.Printf(" (%s)", user.MailAddress.Value)
		}
		fmt.Println()
	}

	ctx := context.Background()

	// 10. 認証情報保存（プロファイルに紐づける）
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
	cfg.SetCredential(profileName, cred)

	// 11. 設定が変更された場合はプロファイルに保存
	if configChanged {
		cfg.SetProfileValue(config.LayerUser, profileName, "space", opts.space)
		cfg.SetProfileValue(config.LayerUser, profileName, "domain", opts.domain)

		if err := cfg.Reload(ctx); err != nil {
			return fmt.Errorf("failed to reload config: %w", err)
		}
	}

	// 設定を保存（クレデンシャルと設定変更）
	if err := cfg.Save(ctx); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Logged in to %s.%s\n", opts.space, opts.domain)

	return nil
}

type loginOptions struct {
	domain       string
	space        string
	noBrowser    bool
	callbackPort int
	timeout      int
	reuse        bool
}

func mergeLoginOptions(cfg *config.Store) loginOptions {
	opts := loginOptions{
		domain:       loginDomain,
		space:        loginSpace,
		noBrowser:    loginNoBrowser,
		callbackPort: loginCallbackPort,
		timeout:      loginTimeout,
		reuse:        loginReuse,
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
