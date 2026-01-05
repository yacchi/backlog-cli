package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

// ExitCode はエラーの終了コード
type ExitCode int

const (
	ExitOK       ExitCode = 0
	ExitError    ExitCode = 1
	ExitAuth     ExitCode = 2
	ExitNotFound ExitCode = 3
	ExitConfig   ExitCode = 4
)

// HandleError はエラーを処理して適切なメッセージを表示する
func HandleError(err error) ExitCode {
	if err == nil {
		return ExitOK
	}

	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		return handleAPIError(apiErr)
	}

	// 一般的なエラー
	ui.Error("%v", err)
	return ExitError
}

func handleAPIError(err *api.APIError) ExitCode {
	switch err.StatusCode {
	case 401:
		ui.Error("Authentication required. Run 'backlog auth login' to authenticate.")
		return ExitAuth
	case 403:
		ui.Error("Permission denied: %s", getErrorMessage(err))
		return ExitAuth
	case 404:
		ui.Error("Not found: %s", getErrorMessage(err))
		return ExitNotFound
	case 429:
		ui.Error("Rate limit exceeded. Please wait and try again.")
		return ExitError
	default:
		ui.Error("API error (%d): %s", err.StatusCode, getErrorMessage(err))
		return ExitError
	}
}

func getErrorMessage(err *api.APIError) string {
	if len(err.Errors) > 0 {
		return err.Errors[0].Message
	}
	return fmt.Sprintf("status %d", err.StatusCode)
}

// PrintError はエラーを標準エラー出力に表示する
func PrintError(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
}
