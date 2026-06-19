package cmdutil

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

// DownloadFunc は添付ファイルのダウンロード関数シグネチャ
type DownloadFunc func(ctx context.Context, w io.Writer) (filename string, size int64, err error)

// DownloadRedirect は BACKLOG_DOWNLOAD_MODE=redirect 時に出力するメタデータ
type DownloadRedirect struct {
	Download bool   `json:"__download"`
	APIPath  string `json:"apiPath"`
	Filename string `json:"filename"`
	OutPath  string `json:"outPath,omitempty"`
}

// RunAttachmentDownload は共通のダウンロード実行ロジック
// BACKLOG_DOWNLOAD_MODE=redirect の場合、ダウンロードせずメタデータ JSON を出力（MCP サーバー用）
// outputFlag == "-" → stdout、outputFlag != "" → 指定パス、空の場合 → fallbackName またはサーバー応答ファイル名
func RunAttachmentDownload(ctx context.Context, outputFlag string, linkMode bool, fallbackName string, apiPath string, dl DownloadFunc) error {
	if linkMode || os.Getenv("BACKLOG_DOWNLOAD_MODE") == "redirect" {
		return outputRedirect(apiPath, outputFlag, fallbackName)
	}

	useStdout := outputFlag == "-"

	if useStdout {
		_, _, err := dl(ctx, os.Stdout)
		return err
	}

	tmpFile, err := os.CreateTemp("", "backlog-attachment-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
	}()

	pw := ui.NewProgressWriter(tmpFile, "Downloading", 0)
	filename, _, err := dl(ctx, pw)
	pw.Finish()
	if err != nil {
		return fmt.Errorf("failed to download attachment: %w", err)
	}
	_ = tmpFile.Close()

	outPath := outputFlag
	if outPath == "" {
		if filename != "" {
			outPath = filename
		} else {
			outPath = fallbackName
		}
	}

	if err := os.Rename(tmpName, outPath); err != nil {
		if copyErr := copyFile(tmpName, outPath); copyErr != nil {
			return fmt.Errorf("failed to save attachment: %w", copyErr)
		}
	}

	abs, _ := filepath.Abs(outPath)
	ui.Success("Downloaded: %s", abs)
	return nil
}

func outputRedirect(apiPath, outputFlag, fallbackName string) error {
	filename := fallbackName
	if outputFlag != "" && outputFlag != "-" {
		filename = filepath.Base(outputFlag)
	}
	meta := DownloadRedirect{
		Download: true,
		APIPath:  apiPath,
		Filename: filename,
		OutPath:  outputFlag,
	}
	return json.NewEncoder(os.Stdout).Encode(meta)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	buf := make([]byte, 32*1024)
	for {
		n, readErr := in.Read(buf)
		if n > 0 {
			if _, writeErr := out.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return readErr
		}
	}
	return nil
}

// IsMCPMode は MCP サーバー経由で実行されているかを返す
func IsMCPMode() bool {
	return os.Getenv("BACKLOG_MCP_MODE") == "1"
}
