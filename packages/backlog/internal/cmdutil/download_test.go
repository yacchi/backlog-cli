package cmdutil

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAttachmentDownload_stdout(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := RunAttachmentDownload(context.Background(), "-", "fallback", func(_ context.Context, out io.Writer) (string, int64, error) {
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

	err := RunAttachmentDownload(context.Background(), outPath, "fallback", func(_ context.Context, w io.Writer) (string, int64, error) {
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

	err := RunAttachmentDownload(context.Background(), "", "fallback.bin", func(_ context.Context, w io.Writer) (string, int64, error) {
		n, e := w.Write([]byte("content"))
		return "server.txt", int64(n), e
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, readErr := os.ReadFile("server.txt")
	if readErr != nil {
		t.Fatalf("file not created: %v", readErr)
	}
	if string(b) != "content" {
		t.Errorf("file content got %q, want %q", string(b), "content")
	}
}

func TestRunAttachmentDownload_fallbackName(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(orig) }()

	err := RunAttachmentDownload(context.Background(), "", "fallback.bin", func(_ context.Context, w io.Writer) (string, int64, error) {
		n, e := w.Write([]byte("bytes"))
		return "", int64(n), e
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, readErr := os.ReadFile("fallback.bin")
	if readErr != nil {
		t.Fatalf("file not created: %v", readErr)
	}
	if !strings.Contains(string(b), "bytes") {
		t.Errorf("unexpected file content: %q", string(b))
	}
}
