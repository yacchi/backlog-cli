package auth

import (
	"strings"
	"testing"
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
