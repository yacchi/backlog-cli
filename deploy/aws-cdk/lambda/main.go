package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/yacchi/backlog-cli/internal/relay"
	"github.com/yacchi/lambda-http-adaptor"
	_ "github.com/yacchi/lambda-http-adaptor/all"

	backlogConfig "github.com/yacchi/backlog-cli/internal/config"
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

// ParameterStoreConfig は Parameter Store から読み込む設定の構造
type ParameterStoreConfig struct {
	CookieSecret        string   `json:"cookieSecret"`
	AllowedSpaces       []string `json:"allowedSpaces,omitempty"`
	AllowedProjects     []string `json:"allowedProjects,omitempty"`
	AllowedHostPatterns string   `json:"allowedHostPatterns,omitempty"`
	Audit               *struct {
		Enabled bool `json:"enabled"`
	} `json:"audit,omitempty"`
	Backlog struct {
		JP *struct {
			ClientID     string `json:"clientId"`
			ClientSecret string `json:"clientSecret"`
		} `json:"jp,omitempty"`
		COM *struct {
			ClientID     string `json:"clientId"`
			ClientSecret string `json:"clientSecret"`
		} `json:"com,omitempty"`
	} `json:"backlog"`
}

func buildHandler() (http.Handler, error) {
	ctx := context.Background()

	// Parameter Store からの読み込みを試行
	if paramName := os.Getenv("BACKLOG_CONFIG_PARAMETER"); paramName != "" {
		if err := loadFromParameterStore(ctx, paramName); err != nil {
			return nil, fmt.Errorf("failed to load config from parameter store: %w", err)
		}
		slog.Info("loaded config from parameter store", "parameter", paramName)
	}

	// Lambda 環境変数から Backlog アプリ設定を正規化
	// jubako のマップキー展開が環境変数で動作しないため、標準形式に変換
	normalizeBacklogEnvVars()

	// 設定をロード（環境変数から）
	cfg, err := backlogConfig.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// 設定をログ出力（センシティブ値はマスクされる）
	logConfig(cfg)

	// リレーサーバーのハンドラーを作成
	server, err := relay.NewServer(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create relay server: %w", err)
	}

	return server.Handler(), nil
}

// normalizeBacklogEnvVars は Lambda 固有の環境変数を jubako が期待する形式に変換する
// CDK/Parameter Store から設定される環境変数を、jubako の env レイヤーが認識できる形式に正規化する
// jubako の環境変数マッチングは大文字小文字を区別しないため、設定さえあれば動作する
func normalizeBacklogEnvVars() {
	// 現時点では正規化不要（jubako が大文字小文字両方に対応）
	// 将来的に必要な変換があればここに追加
}

// loadFromParameterStore は Parameter Store から設定を読み込み、環境変数に設定する
func loadFromParameterStore(ctx context.Context, paramName string) error {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := ssm.NewFromConfig(cfg)

	output, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           &paramName,
		WithDecryption: boolPtr(true),
	})
	if err != nil {
		return fmt.Errorf("failed to get parameter: %w", err)
	}

	if output.Parameter == nil || output.Parameter.Value == nil {
		return fmt.Errorf("parameter value is empty")
	}

	var psc ParameterStoreConfig
	if err := json.Unmarshal([]byte(*output.Parameter.Value), &psc); err != nil {
		return fmt.Errorf("failed to parse parameter JSON: %w", err)
	}

	// 環境変数に設定
	setEnvIfNotEmpty("BACKLOG_COOKIE_SECRET", psc.CookieSecret)

	if psc.Backlog.JP != nil {
		setEnvIfNotEmpty("BACKLOG_CLIENT_ID_JP", psc.Backlog.JP.ClientID)
		setEnvIfNotEmpty("BACKLOG_CLIENT_SECRET_JP", psc.Backlog.JP.ClientSecret)
	}

	if psc.Backlog.COM != nil {
		setEnvIfNotEmpty("BACKLOG_CLIENT_ID_COM", psc.Backlog.COM.ClientID)
		setEnvIfNotEmpty("BACKLOG_CLIENT_SECRET_COM", psc.Backlog.COM.ClientSecret)
	}

	if len(psc.AllowedSpaces) > 0 {
		setEnvIfNotEmpty("BACKLOG_ALLOWED_SPACES", joinStrings(psc.AllowedSpaces))
	}

	if len(psc.AllowedProjects) > 0 {
		setEnvIfNotEmpty("BACKLOG_ALLOWED_PROJECTS", joinStrings(psc.AllowedProjects))
	}

	setEnvIfNotEmpty("BACKLOG_ALLOWED_HOST_PATTERNS", psc.AllowedHostPatterns)

	if psc.Audit != nil {
		if psc.Audit.Enabled {
			os.Setenv("BACKLOG_AUDIT_ENABLED", "true")
		} else {
			os.Setenv("BACKLOG_AUDIT_ENABLED", "false")
		}
	}

	return nil
}

func setEnvIfNotEmpty(key, value string) {
	if value != "" {
		os.Setenv(key, value)
	}
}

func joinStrings(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += ","
		}
		result += s
	}
	return result
}

func boolPtr(b bool) *bool {
	return &b
}

// logConfig は設定をINFOログに出力する（センシティブ値はマスクされる）
// デフォルトレイヤーの設定は除外し、実際に設定された値のみ出力する
func logConfig(cfg *backlogConfig.Store) {
	configMap := make(map[string]any)

	cfg.Walk(func(path string, value any, layerName string) bool {
		// デフォルトレイヤーは除外
		if layerName == backlogConfig.LayerDefaults {
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
