package config

import (
	"testing"
)

func TestFindTrustedBundleByName(t *testing.T) {
	ctx := t.Context()

	store, err := newConfigStore()
	if err != nil {
		t.Fatalf("newConfigStore failed: %v", err)
	}

	if err := store.LoadAll(ctx); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	t.Run("nil store returns nil", func(t *testing.T) {
		result := FindTrustedBundleByName(nil, "test")
		if result != nil {
			t.Errorf("expected nil, got %+v", result)
		}
	})

	t.Run("empty bundles returns nil", func(t *testing.T) {
		result := FindTrustedBundleByName(store, "nonexistent")
		if result != nil {
			t.Errorf("expected nil for nonexistent name, got %+v", result)
		}
	})

	t.Run("find existing bundle", func(t *testing.T) {
		// Add a test bundle
		testBundle := TrustedBundle{
			Name:        "ai2-platform",
			RelayURL:    "https://relay.example.com",
			BundleToken: "test-token",
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

		result := FindTrustedBundleByName(store, "ai2-platform")
		if result == nil {
			t.Fatal("expected to find bundle, got nil")
			return
		}
		if result.ResolvedName() != "ai2-platform" {
			t.Errorf("Name = %q, want %q", result.ResolvedName(), "ai2-platform")
		}
		if result.RelayURL != "https://relay.example.com" {
			t.Errorf("RelayURL = %q, want %q", result.RelayURL, "https://relay.example.com")
		}
	})

	t.Run("v1 bundle resolves name from allowed_domain", func(t *testing.T) {
		// 旧 config 互換: name が無く allowed_domain だけのバンドル
		legacy := TrustedBundle{
			LegacyID:            "legacy.backlog.jp",
			LegacyAllowedDomain: "legacy.backlog.jp",
			RelayURL:            "https://legacy.example.com",
			RelayKeys:           []TrustedRelayKey{{KeyID: "k1", Thumbprint: "abc"}},
		}
		if err := store.Set("client.trust.bundles", []TrustedBundle{legacy}); err != nil {
			t.Fatalf("failed to set legacy bundle: %v", err)
		}
		result := FindTrustedBundleByName(store, "legacy.backlog.jp")
		if result == nil {
			t.Fatal("expected to find legacy bundle by allowed_domain, got nil")
			return
		}
		if result.RelayURL != "https://legacy.example.com" {
			t.Errorf("RelayURL = %q, want %q", result.RelayURL, "https://legacy.example.com")
		}
	})

	t.Run("find by relay url", func(t *testing.T) {
		b := TrustedBundle{
			Name:      "by-url",
			RelayURL:  "https://by-url.example.com",
			RelayKeys: []TrustedRelayKey{{KeyID: "k1", Thumbprint: "abc"}},
		}
		if err := store.Set("client.trust.bundles", []TrustedBundle{b}); err != nil {
			t.Fatalf("failed to set bundle: %v", err)
		}
		result := FindTrustedBundleByRelayURL(store, "https://by-url.example.com")
		if result == nil || result.ResolvedName() != "by-url" {
			t.Fatalf("expected to find bundle by relay url, got %+v", result)
		}
	})

	t.Run("find returns copy not reference", func(t *testing.T) {
		testBundle := TrustedBundle{
			Name:      "copy",
			RelayURL:  "https://original.example.com",
			RelayKeys: []TrustedRelayKey{{KeyID: "k1", Thumbprint: "abc"}},
		}
		err := store.Set("client.trust.bundles", []TrustedBundle{testBundle})
		if err != nil {
			t.Fatalf("failed to set test bundle: %v", err)
		}

		result := FindTrustedBundleByName(store, "copy")
		if result == nil {
			t.Fatal("expected to find bundle, got nil")
			return
		}

		// Modify the returned bundle
		result.RelayURL = "https://modified.example.com"

		// Find again and verify it's unchanged
		result2 := FindTrustedBundleByName(store, "copy")
		if result2 == nil {
			t.Fatal("expected to find bundle again, got nil")
			return
		}
		if result2.RelayURL != "https://original.example.com" {
			t.Errorf("bundle should be unchanged, got RelayURL = %q", result2.RelayURL)
		}
	})
}
