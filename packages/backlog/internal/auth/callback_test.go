package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
)

func TestGenerateState(t *testing.T) {
	state1, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState failed: %v", err)
	}

	if len(state1) == 0 {
		t.Error("GenerateState returned empty string")
	}

	// Should be base64 URL encoded (+ and / should not appear, = is padding)
	if strings.ContainsAny(state1, "+/") {
		t.Error("GenerateState should return URL-safe base64")
	}

	// Should generate unique values
	state2, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState failed: %v", err)
	}

	if state1 == state2 {
		t.Error("GenerateState should generate unique values")
	}
}

func TestParseSpaceHost(t *testing.T) {
	tests := []struct {
		name       string
		spaceHost  string
		wantSpace  string
		wantDomain string
		wantErr    bool
	}{
		{
			name:       "backlog.jp",
			spaceHost:  "myspace.backlog.jp",
			wantSpace:  "myspace",
			wantDomain: "backlog.jp",
			wantErr:    false,
		},
		{
			name:       "backlog.com",
			spaceHost:  "company.backlog.com",
			wantSpace:  "company",
			wantDomain: "backlog.com",
			wantErr:    false,
		},
		{
			name:       "backlogtool.com",
			spaceHost:  "oldspace.backlogtool.com",
			wantSpace:  "oldspace",
			wantDomain: "backlogtool.com",
			wantErr:    false,
		},
		{
			name:       "with https prefix",
			spaceHost:  "https://space.backlog.jp",
			wantSpace:  "space",
			wantDomain: "backlog.jp",
			wantErr:    false,
		},
		{
			name:       "with trailing slash",
			spaceHost:  "space.backlog.jp/",
			wantSpace:  "space",
			wantDomain: "backlog.jp",
			wantErr:    false,
		},
		{
			name:       "with path",
			spaceHost:  "space.backlog.jp/projects",
			wantSpace:  "space",
			wantDomain: "backlog.jp",
			wantErr:    false,
		},
		{
			name:       "with whitespace",
			spaceHost:  "  space.backlog.jp  ",
			wantSpace:  "space",
			wantDomain: "backlog.jp",
			wantErr:    false,
		},
		{
			name:      "unsupported domain",
			spaceHost: "space.example.com",
			wantErr:   true,
		},
		{
			name:      "no subdomain",
			spaceHost: "backlog.jp",
			wantErr:   true,
		},
		{
			name:      "empty",
			spaceHost: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			space, domain, err := parseSpaceHost(tt.spaceHost)

			if tt.wantErr {
				if err == nil {
					t.Error("parseSpaceHost should have returned an error")
				}
				return
			}

			if err != nil {
				t.Fatalf("parseSpaceHost failed: %v", err)
			}

			if space != tt.wantSpace {
				t.Errorf("space = %s, want %s", space, tt.wantSpace)
			}
			if domain != tt.wantDomain {
				t.Errorf("domain = %s, want %s", domain, tt.wantDomain)
			}
		})
	}
}

func TestIsJapanesePreferred(t *testing.T) {
	tests := []struct {
		name           string
		acceptLanguage string
		want           bool
	}{
		{
			name:           "Japanese first",
			acceptLanguage: "ja,en-US;q=0.9,en;q=0.8",
			want:           true,
		},
		{
			name:           "Japanese with region first",
			acceptLanguage: "ja-JP,ja;q=0.9,en;q=0.8",
			want:           true,
		},
		{
			name:           "English first",
			acceptLanguage: "en-US,en;q=0.9,ja;q=0.8",
			want:           false,
		},
		{
			name:           "Japanese only",
			acceptLanguage: "ja",
			want:           true,
		},
		{
			name:           "English only",
			acceptLanguage: "en",
			want:           false,
		},
		{
			name:           "Empty",
			acceptLanguage: "",
			want:           false,
		},
		{
			name:           "Complex with Japanese first",
			acceptLanguage: "ja;q=1.0, en-US;q=0.9, en;q=0.8",
			want:           true,
		},
		{
			name:           "Other language first",
			acceptLanguage: "fr-FR,fr;q=0.9,en;q=0.8,ja;q=0.7",
			want:           false,
		},
		{
			name:           "Japanese uppercase",
			acceptLanguage: "JA,en;q=0.8",
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isJapanesePreferred(tt.acceptLanguage)
			if got != tt.want {
				t.Errorf("isJapanesePreferred(%q) = %v, want %v", tt.acceptLanguage, got, tt.want)
			}
		})
	}
}

func TestExtractLanguageTag(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ja", "ja"},
		{"ja-JP", "ja-jp"},
		{"en-US;q=0.9", "en-us"},
		{"JA;q=1.0", "ja"},
		{"EN", "en"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractLanguageTag(tt.input)
			if got != tt.want {
				t.Errorf("extractLanguageTag(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCheckSessionTimeoutUsesConnectTimeoutBeforeFirstStream(t *testing.T) {
	config.ResetConfig()
	defer config.ResetConfig()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := config.Load(t.Context())
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	if err := cfg.Set("/auth/session/check_interval", 1); err != nil {
		t.Fatalf("cfg.Set(check_interval) error = %v", err)
	}
	if err := cfg.Set("/auth/keepalive/connect_timeout", 0); err != nil {
		t.Fatalf("cfg.Set(connect_timeout) error = %v", err)
	}
	if err := cfg.Set("/auth/keepalive/grace_period", 1); err != nil {
		t.Fatalf("cfg.Set(grace_period) error = %v", err)
	}

	cs := &CallbackServer{
		configStore:        cfg,
		result:             make(chan CallbackResult, 1),
		cancelCheck:        make(chan struct{}),
		statusNotify:       make(chan struct{}, 1),
		sessionEstablished: true,
		session: &Session{
			ID:             "test",
			CreatedAt:      time.Now(),
			LastActivityAt: time.Now(),
			Status:         "pending",
		},
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		cs.checkSessionTimeout()
	}()
	defer func() {
		cs.cancelOnce.Do(func() {
			close(cs.cancelCheck)
		})
		<-done
	}()

	select {
	case result := <-cs.result:
		if result.Error == nil {
			t.Fatal("expected timeout error, got nil")
		}
		if !strings.Contains(result.Error.Error(), "browser failed to connect") {
			t.Fatalf("result.Error = %v, want browser failed to connect", result.Error)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for connect timeout result")
	}
}

func TestCheckSessionTimeoutUsesGracePeriodAfterDisconnect(t *testing.T) {
	config.ResetConfig()
	defer config.ResetConfig()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := config.Load(t.Context())
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	if err := cfg.Set("/auth/session/check_interval", 1); err != nil {
		t.Fatalf("cfg.Set(check_interval) error = %v", err)
	}
	if err := cfg.Set("/auth/keepalive/connect_timeout", 0); err != nil {
		t.Fatalf("cfg.Set(connect_timeout) error = %v", err)
	}
	if err := cfg.Set("/auth/keepalive/grace_period", 0); err != nil {
		t.Fatalf("cfg.Set(grace_period) error = %v", err)
	}

	now := time.Now()
	cs := &CallbackServer{
		configStore:        cfg,
		result:             make(chan CallbackResult, 1),
		cancelCheck:        make(chan struct{}),
		statusNotify:       make(chan struct{}, 1),
		sessionEstablished: true,
		session: &Session{
			ID:             "test",
			CreatedAt:      now.Add(-5 * time.Second),
			LastActivityAt: now.Add(-5 * time.Second),
			Status:         "pending",
		},
		// 一度ストリーム接続したあと、いまは全て切れている状態
		streamEverConnected: true,
		activeStreams:       0,
		disconnectedAt:      &now,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		cs.checkSessionTimeout()
	}()
	defer func() {
		cs.cancelOnce.Do(func() {
			close(cs.cancelCheck)
		})
		<-done
	}()

	select {
	case result := <-cs.result:
		if result.Error == nil {
			t.Fatal("expected grace period error, got nil")
		}
		if !strings.Contains(result.Error.Error(), "browser closed or navigated away") {
			t.Fatalf("result.Error = %v, want browser closed or navigated away", result.Error)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for grace period result")
	}

	status, errorMsg := cs.sessionStatus()
	if status != "error" {
		t.Fatalf("session status = %q, want error", status)
	}
	if !strings.Contains(errorMsg, "browser closed or navigated away") {
		t.Fatalf("session error = %q, want browser closed or navigated away", errorMsg)
	}
}

// createSession で新しいセッションを作っても、進行中のストリーム接続状態が
// 壊れないことを確認する。複数タブやリロードで cs.session が差し替わっても
// activeStreams/streamEverConnected/disconnectedAt は保持される。
func TestCreateSessionPreservesStreamState(t *testing.T) {
	config.ResetConfig()
	defer config.ResetConfig()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := config.Load(t.Context())
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	cs := &CallbackServer{
		configStore: cfg,
		result:      make(chan CallbackResult, 1),
	}

	// 1枚目のセッションを生成しストリームを接続
	first, err := cs.createSession()
	if err != nil {
		t.Fatalf("createSession() error = %v", err)
	}
	cs.handleStreamConnect()

	cs.streamMu.Lock()
	if cs.activeStreams != 1 {
		t.Fatalf("activeStreams after 1st connect = %d, want 1", cs.activeStreams)
	}
	if !cs.streamEverConnected {
		t.Fatal("streamEverConnected should be true after handleStreamConnect")
	}
	cs.streamMu.Unlock()

	// 2枚目のセッションを作成（別タブ・リロード相当）
	second, err := cs.createSession()
	if err != nil {
		t.Fatalf("createSession() #2 error = %v", err)
	}
	if first.ID == second.ID {
		t.Fatal("expected different session IDs for first and second sessions")
	}

	// ストリーム状態は引き継がれているはず
	cs.streamMu.Lock()
	if cs.activeStreams != 1 {
		t.Fatalf("activeStreams after 2nd createSession = %d, want 1", cs.activeStreams)
	}
	if !cs.streamEverConnected {
		t.Fatal("streamEverConnected should remain true after createSession")
	}
	cs.streamMu.Unlock()

	// 元のストリームが切れただけでは「全切断」にはならず timeout 起動しない設計だが、
	// この時点で1つしか開いていないので、ここでは1本切ると disconnectedAt が立つ
	cs.handleStreamDisconnect()
	cs.streamMu.Lock()
	if cs.activeStreams != 0 {
		t.Fatalf("activeStreams after disconnect = %d, want 0", cs.activeStreams)
	}
	if cs.disconnectedAt == nil {
		t.Fatal("disconnectedAt should be set after all streams disconnect")
	}
	cs.streamMu.Unlock()
}

// 複数のストリームが並行している場合、1本切れただけでは
// disconnectedAt は立たない（grace_period が起動しない）ことを確認する。
func TestMultipleStreamsConcurrentDisconnect(t *testing.T) {
	config.ResetConfig()
	defer config.ResetConfig()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := config.Load(t.Context())
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	cs := &CallbackServer{
		configStore: cfg,
		result:      make(chan CallbackResult, 1),
	}
	if _, err := cs.createSession(); err != nil {
		t.Fatalf("createSession() error = %v", err)
	}

	// 2本接続
	cs.handleStreamConnect()
	cs.handleStreamConnect()

	cs.streamMu.Lock()
	if cs.activeStreams != 2 {
		t.Fatalf("activeStreams = %d, want 2", cs.activeStreams)
	}
	cs.streamMu.Unlock()

	// 1本切れる
	cs.handleStreamDisconnect()

	cs.streamMu.Lock()
	if cs.activeStreams != 1 {
		t.Fatalf("activeStreams after 1 disconnect = %d, want 1", cs.activeStreams)
	}
	if cs.disconnectedAt != nil {
		t.Fatal("disconnectedAt should NOT be set while another stream is still connected")
	}
	cs.streamMu.Unlock()

	// もう1本も切れる
	cs.handleStreamDisconnect()

	cs.streamMu.Lock()
	if cs.activeStreams != 0 {
		t.Fatalf("activeStreams after 2nd disconnect = %d, want 0", cs.activeStreams)
	}
	if cs.disconnectedAt == nil {
		t.Fatal("disconnectedAt should be set after all streams disconnect")
	}
	cs.streamMu.Unlock()
}
