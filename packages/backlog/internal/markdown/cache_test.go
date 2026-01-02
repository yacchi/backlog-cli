package markdown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildCacheEntryMasksEmail(t *testing.T) {
	result := ConvertResult{ItemType: "issue", ItemID: 1}
	entry := BuildCacheEntry(result, "a@b.com", "out", 10, false)
	if !strings.Contains(entry.InputExcerpt, "***@***") {
		t.Fatalf("expected masked email, got %q", entry.InputExcerpt)
	}
}

func TestAppendCacheWritesFile(t *testing.T) {
	tmp := t.TempDir()
	entry := CacheEntry{
		ItemType: "issue",
		ItemID:   1,
	}
	if err := AppendCache(entry, tmp); err != nil {
		t.Fatalf("AppendCache error: %v", err)
	}

	path := filepath.Join(tmp, "markdown", "events.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cache file: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("expected cache file to have content")
	}
}
