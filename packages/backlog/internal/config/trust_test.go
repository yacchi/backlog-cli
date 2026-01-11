package config

import (
	"testing"
)

func TestFindTrustedBundle(t *testing.T) {
	ctx := t.Context()

	store, err := newConfigStore()
	if err != nil {
		t.Fatalf("newConfigStore failed: %v", err)
	}

	if err := store.LoadAll(ctx); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	t.Run("nil store returns nil", func(t *testing.T) {
		result := FindTrustedBundle(nil, "test.backlog.jp")
		if result != nil {
			t.Errorf("expected nil, got %+v", result)
		}
	})

	t.Run("empty bundles returns nil", func(t *testing.T) {
		result := FindTrustedBundle(store, "nonexistent.backlog.jp")
		if result != nil {
			t.Errorf("expected nil for nonexistent domain, got %+v", result)
		}
	})

	t.Run("find existing bundle", func(t *testing.T) {
		// Add a test bundle
		testBundle := TrustedBundle{
			ID:            "test.backlog.jp",
			RelayURL:      "https://relay.example.com",
			AllowedDomain: "test.backlog.jp",
			BundleToken:   "test-token",
			RelayKeys: []TrustedRelayKey{
				{KeyID: "k1", Thumbprint: "abc"},
			},
			IssuedAt:   "2025-01-01T00:00:00Z",
			ExpiresAt:  "2025-12-31T23:59:59Z",
			ImportedAt: "2025-01-01T00:00:00Z",
		}
		err := store.Set("client.trust.bundles", []TrustedBundle{testBundle})
		if err != nil {
			t.Fatalf("failed to set test bundle: %v", err)
		}

		result := FindTrustedBundle(store, "test.backlog.jp")
		if result == nil {
			t.Fatal("expected to find bundle, got nil")
			return
		}
		if result.AllowedDomain != "test.backlog.jp" {
			t.Errorf("AllowedDomain = %q, want %q", result.AllowedDomain, "test.backlog.jp")
		}
		if result.RelayURL != "https://relay.example.com" {
			t.Errorf("RelayURL = %q, want %q", result.RelayURL, "https://relay.example.com")
		}
	})

	t.Run("find returns copy not reference", func(t *testing.T) {
		testBundle := TrustedBundle{
			ID:            "copy.backlog.jp",
			AllowedDomain: "copy.backlog.jp",
			RelayURL:      "https://original.example.com",
			RelayKeys:     []TrustedRelayKey{{KeyID: "k1", Thumbprint: "abc"}},
		}
		err := store.Set("client.trust.bundles", []TrustedBundle{testBundle})
		if err != nil {
			t.Fatalf("failed to set test bundle: %v", err)
		}

		result := FindTrustedBundle(store, "copy.backlog.jp")
		if result == nil {
			t.Fatal("expected to find bundle, got nil")
			return
		}

		// Modify the returned bundle
		result.RelayURL = "https://modified.example.com"

		// Find again and verify it's unchanged
		result2 := FindTrustedBundle(store, "copy.backlog.jp")
		if result2 == nil {
			t.Fatal("expected to find bundle again, got nil")
			return
		}
		if result2.RelayURL != "https://original.example.com" {
			t.Errorf("bundle should be unchanged, got RelayURL = %q", result2.RelayURL)
		}
	})
}
