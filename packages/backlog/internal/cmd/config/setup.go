package config

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/auth"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
	"golang.org/x/term"
)

var (
	setupRelayURL   string
	setupName       string
	setupPassphrase string
	setupSpace      string
)

var setupCmd = &cobra.Command{
	Use:   "setup [provisioning-key]",
	Short: "Set up CLI using a provisioning key or relay server credentials",
	Long: `Set up the Backlog CLI using a provisioning key obtained from the portal,
or by specifying the relay server URL, tenant name, and passphrase directly.

Mode 1: Provisioning key (from portal)
  backlog config setup eyJhbGci...

Mode 2: Relay server credentials (for automation / curl|sh)
  backlog config setup --relay-url https://relay.example.com --name my-tenant --passphrase secret
  backlog config setup --relay-url https://relay.example.com --name my-tenant --space example.backlog.jp

Environment variables:
  BACKLOG_RELAY_URL    Relay server URL
  BACKLOG_NAME         Tenant name
  BACKLOG_PASSPHRASE   Passphrase for portal authentication
  BACKLOG_SPACE        Space host (e.g. example.backlog.jp)`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSetup,
}

func init() {
	setupCmd.Flags().BoolVar(&noDefaults, "no-defaults", false, "Do not update default profile values")
	setupCmd.Flags().StringVar(&setupRelayURL, "relay-url", "", "Relay server URL")
	setupCmd.Flags().StringVar(&setupName, "name", "", "Tenant name")
	setupCmd.Flags().StringVar(&setupPassphrase, "passphrase", "", "Portal passphrase")
	setupCmd.Flags().StringVar(&setupSpace, "space", "", "Space host (e.g. example.backlog.jp)")
}

func runSetup(cmd *cobra.Command, args []string) error {
	if len(args) == 1 {
		return runSetupWithToken(cmd, args[0])
	}
	return runSetupWithCredentials(cmd)
}

// runSetupWithToken は従来のプロビジョニングキーによるセットアップ
func runSetupWithToken(cmd *cobra.Command, token string) error {
	claims, err := config.DecodeProvisioningToken(token)
	if err != nil {
		return fmt.Errorf("invalid provisioning key: %w", err)
	}

	fmt.Println("Provisioning key information:")
	fmt.Printf("  Space:        %s\n", claims.Space)
	fmt.Printf("  Domain:       %s\n", claims.Domain)
	fmt.Printf("  Relay server: %s\n", claims.RelayURL)
	fmt.Println()

	if !cmdutil.SkipConfirmation(cmd) && term.IsTerminal(int(syscall.Stdin)) {
		approved, err := ui.Confirm("Import configuration from this relay server?", false)
		if err != nil {
			return err
		}
		if !approved {
			return fmt.Errorf("setup cancelled")
		}
		fmt.Println()
	}

	cfg, err := config.Load(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	ui.Info("Downloading and importing config bundle...")

	imported, err := config.ProvisionFromToken(cmd.Context(), cfg, token, config.ProvisionOptions{
		NoDefaults: noDefaults,
	})
	if err != nil {
		return fmt.Errorf("provisioning failed: %w", err)
	}

	if err := cfg.Save(cmd.Context()); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	ui.Success("Setup complete for bundle %s", imported.Name)
	fmt.Printf("  Relay URL:   %s\n", imported.RelayURL)
	fmt.Printf("  Keys:        %d key(s)\n", len(imported.RelayKeys))
	fmt.Printf("  Expires at:  %s\n", imported.ExpiresAt)
	fmt.Println()
	fmt.Println("You can now authenticate with:")
	fmt.Printf("  backlog auth login\n")
	return nil
}

// runSetupWithCredentials はリレーURL+パスフレーズによるセットアップ
func runSetupWithCredentials(cmd *cobra.Command) error {
	interactive := term.IsTerminal(int(syscall.Stdin))

	relayURL := resolveFlag(setupRelayURL, "BACKLOG_RELAY_URL")
	name := resolveFlag(setupName, "BACKLOG_NAME")
	passphrase := resolveFlag(setupPassphrase, "BACKLOG_PASSPHRASE")
	space := resolveFlag(setupSpace, "BACKLOG_SPACE")

	if relayURL == "" {
		if !interactive {
			return fmt.Errorf("--relay-url or BACKLOG_RELAY_URL is required")
		}
		var err error
		relayURL, err = ui.Input("Relay server URL:", "")
		if err != nil {
			return err
		}
		if relayURL == "" {
			return fmt.Errorf("relay URL is required")
		}
	}

	if name == "" {
		if !interactive {
			return fmt.Errorf("--name or BACKLOG_NAME is required")
		}
		var err error
		name, err = ui.Input("Tenant name:", "")
		if err != nil {
			return err
		}
		if name == "" {
			return fmt.Errorf("tenant name is required")
		}
	}

	if passphrase == "" && interactive {
		// テナント情報を取得して認証方法を判断
		portalInfo, infoErr := config.FetchPortalInfo(cmd.Context(), relayURL, name)

		if infoErr == nil && portalInfo.OAuthEnabled {
			// OAuth SSO: space を解決してからブラウザを開く
			if space == "" && portalInfo.DefaultSpace != "" {
				space = portalInfo.DefaultSpace
			}
			if space == "" {
				var err error
				space, err = ui.Input("Space host (e.g. example.backlog.jp):", "")
				if err != nil {
					return err
				}
				if space == "" {
					return fmt.Errorf("space is required")
				}
			}

			provResp, oauthErr := runSetupOAuth(cmd.Context(), relayURL, name, space)
			if oauthErr != nil {
				return oauthErr
			}

			return finishSetup(cmd, relayURL, space, provResp.ProvisioningKey)
		}

		// フォールバック: パスフレーズ認証
		var err error
		passphrase, err = ui.Password("Passphrase:")
		if err != nil {
			return err
		}
		if passphrase == "" {
			return fmt.Errorf("passphrase is required")
		}
	} else if passphrase == "" {
		return fmt.Errorf("--passphrase or BACKLOG_PASSPHRASE is required")
	}

	ui.Info("Requesting provisioning key from relay server...")

	provResp, err := config.RequestProvisioningKey(cmd.Context(), relayURL, name, passphrase)
	if err != nil {
		return fmt.Errorf("failed to obtain provisioning key: %w", err)
	}

	// space の解決: --space > BACKLOG_SPACE > provision レスポンスの default_space > プロンプト
	if space == "" && provResp.DefaultSpace != "" {
		space = provResp.DefaultSpace
	}
	if space == "" {
		if !interactive {
			return fmt.Errorf("--space or BACKLOG_SPACE is required (no default space configured for this tenant)")
		}
		space, err = ui.Input("Space host (e.g. example.backlog.jp):", "")
		if err != nil {
			return err
		}
		if space == "" {
			return fmt.Errorf("space is required")
		}
	}

	return finishSetup(cmd, relayURL, space, provResp.ProvisioningKey)
}

// finishSetup はプロビジョニングキーを使ってバンドルをインポートし、設定を完了する
func finishSetup(cmd *cobra.Command, relayURL, space, provisioningKey string) error {
	interactive := term.IsTerminal(int(syscall.Stdin))

	fmt.Println()
	fmt.Println("Setup information:")
	fmt.Printf("  Relay server: %s\n", relayURL)
	fmt.Printf("  Space:        %s\n", space)
	fmt.Println()

	if !cmdutil.SkipConfirmation(cmd) && interactive {
		approved, err := ui.Confirm("Proceed with setup?", true)
		if err != nil {
			return err
		}
		if !approved {
			return fmt.Errorf("setup cancelled")
		}
		fmt.Println()
	}

	cfg, err := config.Load(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	ui.Info("Downloading and importing config bundle...")

	imported, err := config.ProvisionFromToken(cmd.Context(), cfg, provisioningKey, config.ProvisionOptions{
		NoDefaults: noDefaults,
	})
	if err != nil {
		return fmt.Errorf("provisioning failed: %w", err)
	}

	if space != "" {
		if err := applySpaceDefaults(cfg, space); err != nil {
			return fmt.Errorf("failed to apply space defaults: %w", err)
		}
	}

	if err := cfg.Save(cmd.Context()); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	ui.Success("Setup complete for bundle %s", imported.Name)
	fmt.Printf("  Relay URL:   %s\n", imported.RelayURL)
	fmt.Printf("  Space:       %s\n", space)
	fmt.Printf("  Keys:        %d key(s)\n", len(imported.RelayKeys))
	fmt.Printf("  Expires at:  %s\n", imported.ExpiresAt)
	fmt.Println()
	fmt.Println("You can now authenticate with:")
	fmt.Printf("  backlog auth login\n")
	return nil
}

// runSetupOAuth はブラウザで OAuth SSO を実行し、プロビジョニングキーを取得する
func runSetupOAuth(ctx context.Context, relayURL, name, space string) (*config.ProvisionResponse, error) {
	state, err := auth.GenerateState()
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}

	// 軽量コールバックサーバーを起動
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to start callback server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	type callbackResult struct {
		code string
		err  error
	}
	resultCh := make(chan callbackResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			resultCh <- callbackResult{err: fmt.Errorf("state mismatch")}
			http.Error(w, "State mismatch", http.StatusBadRequest)
			return
		}
		if errParam := r.URL.Query().Get("error"); errParam != "" {
			desc := r.URL.Query().Get("error_description")
			resultCh <- callbackResult{err: fmt.Errorf("%s: %s", errParam, desc)}
			fmt.Fprintf(w, "Authentication failed: %s. You can close this window.", desc)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			resultCh <- callbackResult{err: fmt.Errorf("missing authorization code")}
			http.Error(w, "Missing code", http.StatusBadRequest)
			return
		}
		resultCh <- callbackResult{code: code}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<html><body><h2>Authentication successful</h2><p>You can close this window.</p></body></html>")
	})

	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	// ブラウザでリレーの OAuth 開始 URL を開く
	authURL := fmt.Sprintf("%s/auth/start?space=%s&port=%d&state=%s",
		strings.TrimRight(relayURL, "/"), space, port, state)

	fmt.Println()
	fmt.Println("Open this URL in your browser to authenticate:")
	fmt.Println()
	fmt.Printf("  %s\n", authURL)
	fmt.Println()
	fmt.Println("Waiting for authentication... (press Ctrl+C to cancel)")

	if err := browser.OpenURL(authURL); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not open browser: %v\n", err)
	}

	// コールバック待機
	var result callbackResult
	select {
	case result = <-resultCh:
	case <-ctx.Done():
		return nil, fmt.Errorf("authentication timed out")
	}

	if result.err != nil {
		return nil, fmt.Errorf("authentication failed: %w", result.err)
	}

	// コード → トークン交換
	ui.Info("Exchanging authorization code...")
	client := auth.NewClient(relayURL)
	tokenResp, err := client.ExchangeToken(auth.TokenRequest{
		GrantType: "authorization_code",
		Code:      result.code,
		Space:     space,
		State:     state,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to exchange token: %w", err)
	}

	// トークンでプロビジョニングキーを取得
	ui.Info("Requesting provisioning key...")
	provResp, err := config.RequestProvisioningKeyWithToken(ctx, relayURL, name, tokenResp.AccessToken, space)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain provisioning key: %w", err)
	}

	return provResp, nil
}

// resolveFlag はフラグ値を環境変数でフォールバックする
func resolveFlag(flagValue, envKey string) string {
	if flagValue != "" {
		return flagValue
	}
	return strings.TrimSpace(os.Getenv(envKey))
}

// applySpaceDefaults はスペースホストをデフォルトプロファイルに設定する
func applySpaceDefaults(store *config.Store, spaceHost string) error {
	space, domain, err := config.ParseSpaceHost(spaceHost)
	if err != nil {
		return err
	}
	if err := store.Set("profile.default.space", space); err != nil {
		return fmt.Errorf("failed to set space: %w", err)
	}
	if err := store.Set("profile.default.domain", domain); err != nil {
		return fmt.Errorf("failed to set domain: %w", err)
	}
	return nil
}
