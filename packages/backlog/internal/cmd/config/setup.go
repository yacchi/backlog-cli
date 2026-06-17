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

	authURL := fmt.Sprintf("%s/auth/start?space=%s&port=%d&state=%s",
		strings.TrimRight(relayURL, "/"), space, port, state)

	mux := http.NewServeMux()

	// ランディングページ: 認証前に何の認証か説明する
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, setupLandingHTML, name, space, relayURL, authURL)
	})

	// コールバック: 認証結果を受け取る
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			resultCh <- callbackResult{err: fmt.Errorf("state mismatch")}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(w, setupResultHTML, "認証エラー", "セッションが無効です。もう一度やり直してください。", "#dc2626")
			return
		}
		if errParam := r.URL.Query().Get("error"); errParam != "" {
			desc := r.URL.Query().Get("error_description")
			resultCh <- callbackResult{err: fmt.Errorf("%s: %s", errParam, desc)}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(w, setupResultHTML, "認証に失敗しました", desc, "#dc2626")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			resultCh <- callbackResult{err: fmt.Errorf("missing authorization code")}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(w, setupResultHTML, "認証エラー", "認可コードが見つかりません。", "#dc2626")
			return
		}
		resultCh <- callbackResult{code: code}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, setupResultHTML,
			"認証が完了しました",
			"ターミナルに戻ってセットアップの完了をお待ちください。このタブは閉じて構いません。",
			"#059669")
	})

	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	// ランディングページを開く（直接 Backlog に飛ばさない）
	landingURL := fmt.Sprintf("http://localhost:%d/", port)

	fmt.Println()
	fmt.Println("Open this URL in your browser to authenticate:")
	fmt.Println()
	fmt.Printf("  %s\n", landingURL)
	fmt.Println()
	fmt.Println("Waiting for authentication... (press Ctrl+C to cancel)")

	if err := browser.OpenURL(landingURL); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not open browser: %v\n", err)
	}

	var result callbackResult
	select {
	case result = <-resultCh:
	case <-ctx.Done():
		return nil, fmt.Errorf("authentication timed out")
	}

	if result.err != nil {
		return nil, fmt.Errorf("authentication failed: %w", result.err)
	}

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

	ui.Info("Requesting provisioning key...")
	provResp, err := config.RequestProvisioningKeyWithToken(ctx, relayURL, name, tokenResp.AccessToken, space)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain provisioning key: %w", err)
	}

	return provResp, nil
}

const setupPageStyle = `
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;
background:#f8fafc;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:2rem}
.card{background:#fff;border-radius:1rem;box-shadow:0 1px 3px rgba(0,0,0,.1);
max-width:28rem;width:100%%;padding:2.5rem;text-align:center}
.badge{font-size:.7rem;font-weight:600;text-transform:uppercase;letter-spacing:.15em;color:#6b7280}
h1{font-size:1.5rem;font-weight:600;color:#1e293b;margin:.75rem 0}
p{font-size:.875rem;color:#64748b;line-height:1.6}
.info{margin:1.25rem 0;text-align:left}
.info-row{display:flex;padding:.5rem .75rem;border-radius:.5rem;background:#f1f5f9;margin-bottom:.5rem}
.info-label{font-size:.75rem;font-weight:500;color:#6b7280;min-width:6rem}
.info-value{font-size:.75rem;color:#1e293b;word-break:break-all}
.btn{display:inline-block;padding:.75rem 2rem;border-radius:.75rem;font-size:.875rem;font-weight:500;
text-decoration:none;transition:all .15s;cursor:pointer;border:none}
.btn-primary{background:#2563eb;color:#fff}
.btn-primary:hover{background:#1d4ed8}
.hint{font-size:.75rem;color:#94a3b8;margin-top:1rem}
`

var setupLandingHTML = `<!DOCTYPE html><html lang="ja"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Backlog CLI Setup</title><style>` + setupPageStyle + `</style></head>
<body><div class="card">
<p class="badge">Backlog CLI</p>
<h1>CLIセットアップ認証</h1>
<p>以下の設定でセットアップを行います。<br>Backlog にログインして認証を完了してください。</p>
<div class="info">
<div class="info-row"><span class="info-label">テナント</span><span class="info-value">%s</span></div>
<div class="info-row"><span class="info-label">スペース</span><span class="info-value">%s</span></div>
<div class="info-row"><span class="info-label">リレーサーバー</span><span class="info-value">%s</span></div>
</div>
<a class="btn btn-primary" href="%s">Backlog にログイン</a>
<p class="hint">認証後、自動的にこのページに戻ります</p>
</div></body></html>`

var setupResultHTML = `<!DOCTYPE html><html lang="ja"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Backlog CLI Setup</title><style>` + setupPageStyle + `
.icon{font-size:2.5rem;margin-bottom:.5rem}
</style></head>
<body><div class="card">
<p class="badge">Backlog CLI</p>
<div class="icon" style="color:%[3]s">` + "&#x25cf;" + `</div>
<h1>%[1]s</h1>
<p>%[2]s</p>
</div></body></html>`

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
