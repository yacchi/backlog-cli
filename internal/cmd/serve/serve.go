package serve

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/config"
	"github.com/yacchi/backlog-cli/internal/relay"
)

var ServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the OAuth relay server",
	Long: `Start the OAuth relay server for Backlog CLI authentication.

The relay server handles OAuth 2.0 authentication flow, keeping the
client_id and client_secret secure on the server side.`,
	RunE: runServe,
}

var (
	configFile string
	port       int
)

func init() {
	ServeCmd.Flags().StringVarP(&configFile, "config", "c", "", "Config file path")
	ServeCmd.Flags().IntVar(&port, "port", 0, "Server port (overrides config)")
}

func runServe(cmd *cobra.Command, args []string) error {
	// 設定読み込み
	ctx := cmd.Context()
	cfg, err := config.Load(ctx)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// ポートのオーバーライド（コマンドライン引数）
	if port > 0 {
		_ = cfg.SetToLayer(config.LayerArgs, "server.port", port)
		_ = cfg.Reload(ctx)
	}

	// サーバー作成
	srv, err := relay.NewServer(cfg)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	// シグナルハンドリング
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	select {
	case err := <-errCh:
		return err
	case <-stop:
		fmt.Println("\nShutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}
}
