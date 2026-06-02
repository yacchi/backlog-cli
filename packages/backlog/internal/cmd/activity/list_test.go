package activity

import (
	"testing"
	"time"
)

func TestParseDateRange(t *testing.T) {
	loc := time.UTC
	sinceT, untilT, hasSince, hasUntil, err := parseDateRange("2026-05-26", "2026-06-02", loc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasSince || !hasUntil {
		t.Fatalf("hasSince=%v hasUntil=%v, want both true", hasSince, hasUntil)
	}
	if !sinceT.Equal(time.Date(2026, 5, 26, 0, 0, 0, 0, loc)) {
		t.Fatalf("sinceT = %v", sinceT)
	}
	// until は当日いっぱいを含む
	wantUntil := time.Date(2026, 6, 3, 0, 0, 0, 0, loc).Add(-time.Nanosecond)
	if !untilT.Equal(wantUntil) {
		t.Fatalf("untilT = %v, want %v", untilT, wantUntil)
	}

	if _, _, _, _, err := parseDateRange("bad", "", loc); err == nil {
		t.Fatal("expected error for invalid --since")
	}
}

func TestWithinRange(t *testing.T) {
	loc := time.UTC
	sinceT := time.Date(2026, 5, 26, 0, 0, 0, 0, loc)
	untilT := time.Date(2026, 6, 3, 0, 0, 0, 0, loc).Add(-time.Nanosecond)

	in := time.Date(2026, 5, 27, 12, 0, 0, 0, loc)
	before := time.Date(2026, 5, 25, 12, 0, 0, 0, loc)
	after := time.Date(2026, 6, 4, 0, 0, 0, 0, loc)

	if !withinRange(in, true, sinceT, untilT, true, true) {
		t.Fatal("in-range time should be included")
	}
	if withinRange(before, true, sinceT, untilT, true, true) {
		t.Fatal("before-since time should be excluded")
	}
	if withinRange(after, true, sinceT, untilT, true, true) {
		t.Fatal("after-until time should be excluded")
	}
	// レンジ無しなら常に含む
	if !withinRange(before, true, time.Time{}, time.Time{}, false, false) {
		t.Fatal("with no range, all should be included")
	}
	// パース不能 + レンジ指定ありなら除外
	if withinRange(time.Time{}, false, sinceT, untilT, true, false) {
		t.Fatal("unparseable time with range should be excluded")
	}
}
