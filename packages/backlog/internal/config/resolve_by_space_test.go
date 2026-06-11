package config

import (
	"context"
	"testing"
)

func TestResolveBySpace(t *testing.T) {
	t.Run("single match", func(t *testing.T) {
		t.Setenv("BACKLOG_PROFILE_default_SPACE", "example")
		t.Setenv("BACKLOG_PROFILE_default_DOMAIN", "backlog.jp")

		store := loadTestStore(t)

		name, err := store.ResolveBySpace("example.backlog.jp")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != "default" {
			t.Errorf("got %q, want %q", name, "default")
		}
	})

	t.Run("no match", func(t *testing.T) {
		t.Setenv("BACKLOG_PROFILE_default_SPACE", "example")
		t.Setenv("BACKLOG_PROFILE_default_DOMAIN", "backlog.jp")

		store := loadTestStore(t)

		_, err := store.ResolveBySpace("other.backlog.jp")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("multiple match with primary", func(t *testing.T) {
		t.Setenv("BACKLOG_PROFILE_default_SPACE", "example")
		t.Setenv("BACKLOG_PROFILE_default_DOMAIN", "backlog.jp")
		t.Setenv("BACKLOG_PROFILE_default_PRIMARY", "true")
		t.Setenv("BACKLOG_PROFILE_readonly_SPACE", "example")
		t.Setenv("BACKLOG_PROFILE_readonly_DOMAIN", "backlog.jp")

		store := loadTestStore(t)

		name, err := store.ResolveBySpace("example.backlog.jp")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != "default" {
			t.Errorf("got %q, want %q", name, "default")
		}
	})

	t.Run("multiple match without primary", func(t *testing.T) {
		t.Setenv("BACKLOG_PROFILE_default_SPACE", "example")
		t.Setenv("BACKLOG_PROFILE_default_DOMAIN", "backlog.jp")
		t.Setenv("BACKLOG_PROFILE_default_PRIMARY", "false")
		t.Setenv("BACKLOG_PROFILE_readonly_SPACE", "example")
		t.Setenv("BACKLOG_PROFILE_readonly_DOMAIN", "backlog.jp")
		t.Setenv("BACKLOG_PROFILE_readonly_PRIMARY", "false")

		store := loadTestStore(t)

		_, err := store.ResolveBySpace("example.backlog.jp")
		if err == nil {
			t.Fatal("expected error for multiple profiles without primary, got nil")
		}
	})

	t.Run("invalid format", func(t *testing.T) {
		store := loadTestStore(t)

		_, err := store.ResolveBySpace("invalid")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("different domain not matched", func(t *testing.T) {
		t.Setenv("BACKLOG_PROFILE_default_SPACE", "example")
		t.Setenv("BACKLOG_PROFILE_default_DOMAIN", "backlog.jp")

		store := loadTestStore(t)

		_, err := store.ResolveBySpace("example.backlog.com")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func loadTestStore(t *testing.T) *Store {
	t.Helper()

	ResetConfig()
	t.Cleanup(ResetConfig)

	store, err := newConfigStore()
	if err != nil {
		t.Fatalf("newConfigStore: %v", err)
	}
	if err := store.LoadAll(context.Background()); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	return store
}
