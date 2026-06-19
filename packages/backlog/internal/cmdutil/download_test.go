package cmdutil

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var noop = func(_ context.Context, w io.Writer) (string, int64, error) {
	n, e := w.Write([]byte("data"))
	return "", int64(n), e
}

func TestRunAttachmentDownload_stdout(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := RunAttachmentDownload(context.Background(), "-", false, "fallback", "/issues/X/attachments/1", func(_ context.Context, out io.Writer) (string, int64, error) {
		n, _ := out.Write([]byte("hello"))
		return "", int64(n), nil
	})

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.String() != "hello" {
		t.Errorf("stdout got %q, want %q", buf.String(), "hello")
	}
}

func TestRunAttachmentDownload_explicitPath(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.txt")

	err := RunAttachmentDownload(context.Background(), outPath, false, "fallback", "/issues/X/attachments/1", func(_ context.Context, w io.Writer) (string, int64, error) {
		n, e := w.Write([]byte("data"))
		return "server-name.txt", int64(n), e
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, _ := os.ReadFile(outPath)
	if string(b) != "data" {
		t.Errorf("file content got %q, want %q", string(b), "data")
	}
}

func TestRunAttachmentDownload_serverFilename(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(orig) }()

	err := RunAttachmentDownload(context.Background(), "", false, "fallback.bin", "/issues/X/attachments/1", func(_ context.Context, w io.Writer) (string, int64, error) {
		n, e := w.Write([]byte("content"))
		return "server.txt", int64(n), e
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b, _ := os.ReadFile("server.txt"); string(b) != "content" {
		t.Errorf("file content got %q, want %q", string(b), "content")
	}
}

func TestRunAttachmentDownload_fallbackName(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(orig) }()

	err := RunAttachmentDownload(context.Background(), "", false, "fallback.bin", "/issues/X/attachments/1", func(_ context.Context, w io.Writer) (string, int64, error) {
		n, e := w.Write([]byte("bytes"))
		return "", int64(n), e
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b, _ := os.ReadFile("fallback.bin"); !strings.Contains(string(b), "bytes") {
		t.Errorf("unexpected file content: %q", string(b))
	}
}

func TestRunAttachmentDownload_linkFlag(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := RunAttachmentDownload(context.Background(), "/tmp/my-report.png", true, "fallback.bin", "/issues/PROJ-1/attachments/42", noop)

	_ = w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	var meta DownloadRedirect
	if err := json.Unmarshal(buf.Bytes(), &meta); err != nil {
		t.Fatalf("failed to parse redirect JSON: %v", err)
	}
	if !meta.Download {
		t.Error("__download should be true")
	}
	if meta.APIPath != "/issues/PROJ-1/attachments/42" {
		t.Errorf("apiPath got %q, want %q", meta.APIPath, "/issues/PROJ-1/attachments/42")
	}
	if meta.Filename != "my-report.png" {
		t.Errorf("filename got %q, want %q", meta.Filename, "my-report.png")
	}
}
