package markdown

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const defaultExcerptLength = 200

var emailMask = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)

// AppendCache writes an entry to the JSONL cache.
func AppendCache(entry CacheEntry, cacheDir string) error {
	path, err := cacheFilePath(cacheDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open cache file: %w", err)
	}
	defer func() { _ = file.Close() }()

	enc := json.NewEncoder(file)
	if err := enc.Encode(entry); err != nil {
		return fmt.Errorf("write cache entry: %w", err)
	}
	return nil
}

// BuildCacheEntry creates a cache entry from a conversion result.
func BuildCacheEntry(result ConvertResult, input, output string, excerptLength int, includeRaw bool) CacheEntry {
	if excerptLength <= 0 {
		excerptLength = defaultExcerptLength
	}
	entry := CacheEntry{
		TS:            time.Now(),
		ItemType:      result.ItemType,
		ItemID:        result.ItemID,
		ParentID:      result.ParentID,
		ProjectKey:    result.ProjectKey,
		ItemKey:       result.ItemKey,
		URL:           result.URL,
		DetectedMode:  result.Mode,
		Score:         result.Score,
		Warnings:      result.Warnings,
		WarningLines:  result.WarningLines,
		RulesApplied:  result.Rules,
		InputHash:     sha256Hex(input),
		OutputHash:    sha256Hex(output),
		InputExcerpt:  excerpt(maskIfNeeded(input, includeRaw), excerptLength),
		OutputExcerpt: excerpt(maskIfNeeded(output, includeRaw), excerptLength),
	}

	if includeRaw {
		entry.InputRaw = input
		entry.OutputRaw = output
	}

	return entry
}

func cacheFilePath(cacheDir string) (string, error) {
	if cacheDir != "" {
		return filepath.Join(cacheDir, "markdown", "events.jsonl"), nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "backlog", "markdown", "events.jsonl"), nil
}

func sha256Hex(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}

func excerpt(input string, length int) string {
	trimmed := strings.TrimSpace(input)
	if len(trimmed) <= length {
		return trimmed
	}
	return trimmed[:length]
}

func maskIfNeeded(input string, includeRaw bool) string {
	if includeRaw {
		return input
	}
	return emailMask.ReplaceAllString(input, "***@***")
}
