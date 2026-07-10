package cache

import (
	"context"
	"testing"
	"time"
)

func TestFileCache_Delete(t *testing.T) {
	dir := t.TempDir()
	c, err := NewFileCache(dir)
	if err != nil {
		t.Fatal(err)
	}

	key := "issue:backlog.jp:PROJ-1"
	if err := c.Set(key, "hello", 5*time.Minute); err != nil {
		t.Fatal(err)
	}

	var v string
	ok, err := c.Get(key, &v)
	if err != nil || !ok {
		t.Fatal("expected cache hit")
	}

	if err := c.Delete(key); err != nil {
		t.Fatal(err)
	}

	ok, err = c.Get(key, &v)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected cache miss after Delete")
	}
}

func TestFileCache_Delete_NonExistent(t *testing.T) {
	dir := t.TempDir()
	c, err := NewFileCache(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := c.Delete("nonexistent:key"); err != nil {
		t.Fatalf("Delete of non-existent key should not error: %v", err)
	}
}

func TestFileCache_DeleteByPrefix(t *testing.T) {
	dir := t.TempDir()
	c, err := NewFileCache(dir)
	if err != nil {
		t.Fatal(err)
	}

	keys := []string{
		"comments:backlog.jp:PROJ-1:order=asc",
		"comments:backlog.jp:PROJ-1:order=desc",
		"comments:backlog.jp:PROJ-2:order=asc",
		"issue:backlog.jp:PROJ-1",
	}
	for _, k := range keys {
		if err := c.Set(k, "data", 5*time.Minute); err != nil {
			t.Fatal(err)
		}
	}

	if err := c.DeleteByPrefix("comments:backlog.jp:PROJ-1:"); err != nil {
		t.Fatal(err)
	}

	var v string

	for _, k := range keys[:2] {
		ok, _ := c.Get(k, &v)
		if ok {
			t.Fatalf("expected cache miss for %s after DeleteByPrefix", k)
		}
	}

	for _, k := range keys[2:] {
		ok, _ := c.Get(k, &v)
		if !ok {
			t.Fatalf("expected cache hit for %s (should not be affected by DeleteByPrefix)", k)
		}
	}
}

func TestFileCache_Cleanup(t *testing.T) {
	dir := t.TempDir()
	c, err := NewFileCache(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := c.Set("key1", "data", 1*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)

	if err := c.Cleanup(context.Background(), 0); err != nil {
		t.Fatal(err)
	}

	var v string
	ok, _ := c.Get("key1", &v)
	if ok {
		t.Fatal("expected cache miss after Cleanup")
	}
}
