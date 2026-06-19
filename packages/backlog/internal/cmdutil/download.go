package cmdutil

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

// DownloadFunc は添付ファイルのダウンロード関数シグネチャ
type DownloadFunc func(ctx context.Context, w io.Writer) (filename string, size int64, err error)

// RunAttachmentDownload は共通のダウンロード実行ロジック
// BACKLOG_OUTPUT_DIR が設定されている場合、そのディレクトリに保存（MCP サーバー用）
// outputFlag == "-" → stdout、outputFlag != "" → 指定パス、空の場合 → fallbackName またはサーバー応答ファイル名
func RunAttachmentDownload(ctx context.Context, outputFlag string, fallbackName string, dl DownloadFunc) error {
	if outputDir := os.Getenv("BACKLOG_OUTPUT_DIR"); outputDir != "" {
		return downloadToDir(ctx, outputDir, outputFlag, fallbackName, dl)
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

	filename, _, err := dl(ctx, tmpFile)
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

func downloadToDir(ctx context.Context, dir, outputFlag, fallbackName string, dl DownloadFunc) error {
	tmpFile, err := os.CreateTemp("", "backlog-attachment-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
	}()

	filename, _, err := dl(ctx, tmpFile)
	if err != nil {
		return fmt.Errorf("failed to download attachment: %w", err)
	}
	_ = tmpFile.Close()

	var outPath string
	if outputFlag != "" && outputFlag != "-" {
		// -o のパス構造を output dir 配下にそのまま再現
		p := outputFlag
		if filepath.IsAbs(p) {
			p = p[1:]
		}
		outPath = filepath.Join(dir, p)
	} else {
		name := fallbackName
		if filename != "" {
			name = filepath.Base(filename)
		}
		outPath = filepath.Join(dir, name)
	}
	_ = os.MkdirAll(filepath.Dir(outPath), 0o755)

	if err := os.Rename(tmpName, outPath); err != nil {
		if copyErr := copyFile(tmpName, outPath); copyErr != nil {
			return fmt.Errorf("failed to save attachment: %w", copyErr)
		}
	}

	ui.Success("Downloaded: %s", outPath)
	return nil
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
