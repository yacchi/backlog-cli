package ui

import (
	"fmt"
	"io"
	"os"
	"sync"
)

// ProgressReader は io.Reader をラップし、stderr に転送量を表示する
// TTY でない場合は表示をスキップする
type ProgressReader struct {
	r       io.Reader
	label   string
	total   int64 // 0 の場合は不明
	current int64
	mu      sync.Mutex
}

// NewProgressReader は進捗表示付きの io.Reader を返す
// total が 0 の場合はサイズ不明として転送バイト数のみ表示する
func NewProgressReader(r io.Reader, label string, total int64) *ProgressReader {
	return &ProgressReader{r: r, label: label, total: total}
}

func (p *ProgressReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	if n > 0 {
		p.mu.Lock()
		p.current += int64(n)
		p.printProgress()
		p.mu.Unlock()
	}
	if err == io.EOF {
		p.clearLine()
	}
	return n, err
}

// ProgressWriter は io.Writer をラップし、stderr に転送量を表示する
type ProgressWriter struct {
	w       io.Writer
	label   string
	total   int64
	current int64
	mu      sync.Mutex
}

// NewProgressWriter は進捗表示付きの io.Writer を返す
func NewProgressWriter(w io.Writer, label string, total int64) *ProgressWriter {
	return &ProgressWriter{w: w, label: label, total: total}
}

func (p *ProgressWriter) Write(buf []byte) (int, error) {
	n, err := p.w.Write(buf)
	if n > 0 {
		p.mu.Lock()
		p.current += int64(n)
		p.printProgress()
		p.mu.Unlock()
	}
	return n, err
}

// Finish は進捗行をクリアする
func (p *ProgressWriter) Finish() {
	clearLine()
}

func (p *ProgressReader) printProgress() {
	if !IsStderrTTY() {
		return
	}
	if p.total > 0 {
		pct := float64(p.current) / float64(p.total) * 100
		fmt.Fprintf(os.Stderr, "\r  %s %s / %s (%.0f%%)", p.label, formatSize(p.current), formatSize(p.total), pct)
	} else {
		fmt.Fprintf(os.Stderr, "\r  %s %s", p.label, formatSize(p.current))
	}
}

func (p *ProgressReader) clearLine() {
	clearLine()
}

func (p *ProgressWriter) printProgress() {
	if !IsStderrTTY() {
		return
	}
	if p.total > 0 {
		pct := float64(p.current) / float64(p.total) * 100
		fmt.Fprintf(os.Stderr, "\r  %s %s / %s (%.0f%%)", p.label, formatSize(p.current), formatSize(p.total), pct)
	} else {
		fmt.Fprintf(os.Stderr, "\r  %s %s", p.label, formatSize(p.current))
	}
}

func clearLine() {
	if !IsStderrTTY() {
		return
	}
	fmt.Fprint(os.Stderr, "\r\033[K")
}

func formatSize(b int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1fGB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1fMB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1fKB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%dB", b)
	}
}
