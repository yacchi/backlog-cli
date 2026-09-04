package ratelimit

import (
	"strings"
	"testing"
	"time"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
)

func TestRateLimitRowsOrder(t *testing.T) {
	r := &api.RateLimit{
		Read:   api.RateLimitCategory{Limit: 600, Remaining: 599, Reset: 1500000000},
		Update: api.RateLimitCategory{Limit: 150, Remaining: 0, Reset: 1500000100},
		Search: api.RateLimitCategory{Limit: 150, Remaining: 149, Reset: 1500000200},
		Icon:   api.RateLimitCategory{Limit: 150, Remaining: 149, Reset: 1500000300},
	}

	rows := rateLimitRows(r)
	wantOrder := []string{"read", "update", "search", "icon"}
	if len(rows) != len(wantOrder) {
		t.Fatalf("len(rows) = %d, want %d", len(rows), len(wantOrder))
	}
	for i, name := range wantOrder {
		if rows[i].Category != name {
			t.Fatalf("rows[%d].Category = %q, want %q", i, rows[i].Category, name)
		}
	}
	if rows[1].Value.Remaining != 0 {
		t.Fatalf("update.remaining = %d, want 0", rows[1].Value.Remaining)
	}
}

func TestFormatResetLocal(t *testing.T) {
	reset := time.Date(2026, 6, 15, 12, 30, 45, 0, time.UTC).Unix()

	got := formatResetLocal(reset, "UTC")
	want := "2026-06-15 12:30:45"
	if got != want {
		t.Fatalf("formatResetLocal() = %q, want %q", got, want)
	}

	// 不正なタイムゾーンはローカルタイムにフォールバックする（パニックしない）
	got = formatResetLocal(reset, "Not/A_Real_Zone")
	if got == "" {
		t.Fatal("formatResetLocal() with invalid timezone returned empty string")
	}
}

func TestRenderRateLimitTableContainsAllCategories(t *testing.T) {
	r := &api.RateLimit{
		Read:   api.RateLimitCategory{Limit: 600, Remaining: 599, Reset: 1500000000},
		Update: api.RateLimitCategory{Limit: 150, Remaining: 0, Reset: 1500000100},
		Search: api.RateLimitCategory{Limit: 150, Remaining: 149, Reset: 1500000200},
		Icon:   api.RateLimitCategory{Limit: 150, Remaining: 149, Reset: 1500000300},
	}

	out := captureStdout(t, func() {
		renderRateLimitTable(r, "UTC")
	})

	for _, want := range []string{"read", "update", "search", "icon", "600", "599", "0", "149"} {
		if !strings.Contains(out, want) {
			t.Fatalf("table output missing %q: %s", want, out)
		}
	}
}

func TestRateLimitJSONOutputKeepsRawResetNumber(t *testing.T) {
	r := &api.RateLimit{
		Read: api.RateLimitCategory{Limit: 600, Remaining: 599, Reset: 1500000000},
	}

	out := captureStdout(t, func() {
		if err := cmdutil.OutputJSONFromProfile(r, "", "", ""); err != nil {
			t.Fatalf("OutputJSONFromProfile returned error: %v", err)
		}
	})

	if !strings.Contains(out, `"reset": 1500000000`) {
		t.Fatalf("json output should keep reset as a raw number: %s", out)
	}
}
