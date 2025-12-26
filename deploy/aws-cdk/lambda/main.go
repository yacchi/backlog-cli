package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/yacchi/jubako/format/json"
	"github.com/yacchi/jubako/source/aws"
	"github.com/yacchi/lambda-http-adaptor"
	_ "github.com/yacchi/lambda-http-adaptor/all"

	backlog "github.com/yacchi/backlog-cli"
)

func main() {
	// structured logging for CloudWatch
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	handler, err := buildHandler()
	if err != nil {
		slog.Error("failed to build handler", "error", err)
		os.Exit(1)
	}

	slog.Info("starting lambda handler")
	if err := adaptor.ListenAndServe("", handler); err != nil {
		slog.Error("handler error", "error", err)
		os.Exit(1)
	}
}

func buildHandler() (http.Handler, error) {
	ctx := context.Background()

	// 設定をロード
	cfg, err := backlog.LoadConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Parameter Store からの読み込みを試行
	if paramName := os.Getenv("BACKLOG_CONFIG_PARAMETER"); paramName != "" {
		src := aws.NewParameterStoreSource(paramName, aws.WithDecryption(true))
		if err := cfg.AddParameterStoreLayer(ctx, src, json.New()); err != nil {
			return nil, fmt.Errorf("failed to apply parameter store config: %w", err)
		}
		slog.Info("loaded config from parameter store", "parameter", paramName)
	}

	// 設定をログ出力（センシティブ値はマスクされる）
	logConfig(cfg)

	// リレーサーバーのハンドラーを作成
	handler, err := backlog.AuthRelayHandler(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create relay server: %w", err)
	}

	return handler, nil
}

// logConfig は設定をINFOログに出力する（センシティブ値はマスクされる）
// デフォルトレイヤーの設定は除外し、実際に設定された値のみ出力する
func logConfig(cfg *backlog.Config) {
	configMap := make(map[string]any)

	cfg.Walk(func(path string, value any, layerName string) bool {
		// デフォルトレイヤーは除外
		if layerName == "defaults" {
			return true
		}
		configMap[path] = map[string]any{
			"value": value,
			"layer": layerName,
		}
		return true
	})

	slog.Info("configuration loaded", "config", configMap)
}
