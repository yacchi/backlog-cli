package auth

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/config"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show authentication status",
	Long: `Show authentication status for all configured accounts.

Examples:
  backlog auth status
  backlog auth status --quiet`,
	RunE: runStatus,
}

var statusQuiet bool

func init() {
	statusCmd.Flags().BoolVarP(&statusQuiet, "quiet", "q", false, "Exit with code 0 if credentials exist, 1 otherwise (no output)")
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cmd.Context())
	if err != nil {
		if statusQuiet {
			os.Exit(1)
		}
		return fmt.Errorf("failed to load config: %w", err)
	}

	resolved := cfg.Resolved()
	credentials := resolved.Credentials
	if len(credentials) == 0 {
		if statusQuiet {
			os.Exit(1)
		}
		fmt.Println("Not logged in to any account")
		fmt.Println()
		fmt.Println("Run 'backlog auth login' to authenticate")
		return nil
	}

	// quiet mode: just check if credentials exist, no output
	if statusQuiet {
		return nil
	}

	// プロファイル情報を取得して表示に使用
	profiles := resolved.Profiles

	fmt.Println("Authenticated accounts:")
	fmt.Println()

	for profileName, cred := range credentials {
		// プロファイルからspace.domainを取得
		var host string
		if profile, ok := profiles[profileName]; ok && profile.Space != "" && profile.Domain != "" {
			host = profile.Space + "." + profile.Domain
		} else {
			host = "(not configured)"
		}

		fmt.Printf("  [%s] %s\n", profileName, host)
		if cred.UserName != "" {
			fmt.Printf("    User: %s\n", cred.UserName)
		}

		// 認証タイプの表示
		authType := cred.GetAuthType()
		switch authType {
		case config.AuthTypeAPIKey:
			fmt.Println("    Auth: API Key")
		case config.AuthTypeOAuth:
			fmt.Println("    Auth: OAuth 2.0")
			// OAuthの場合のみトークン有効期限を表示
			printTokenStatus(cred.ExpiresAt)
		default:
			fmt.Println("    Auth: OAuth 2.0")
			printTokenStatus(cred.ExpiresAt)
		}
		fmt.Println()
	}

	return nil
}

// printTokenStatus はトークンの状態を表示する
func printTokenStatus(expiresAt time.Time) {
	if expiresAt.IsZero() {
		fmt.Println("    Token: valid")
	} else if time.Now().After(expiresAt) {
		fmt.Println("    Token: expired (will refresh on next request)")
	} else {
		remaining := time.Until(expiresAt).Round(time.Minute)
		fmt.Printf("    Token: valid (expires in %s)\n", remaining)
	}
}
