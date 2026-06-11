package domain

import (
	"testing"
)

func TestSplitDomain(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantSpace   string
		wantBacklog string
	}{
		{
			name:        "backlog.jp domain",
			input:       "myspace.backlog.jp",
			wantSpace:   "myspace",
			wantBacklog: "backlog.jp",
		},
		{
			name:        "backlog.com domain",
			input:       "myspace.backlog.com",
			wantSpace:   "myspace",
			wantBacklog: "backlog.com",
		},
		{
			name:        "single part",
			input:       "onlyspace",
			wantSpace:   "onlyspace",
			wantBacklog: "",
		},
		{
			name:        "empty string",
			input:       "",
			wantSpace:   "",
			wantBacklog: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSpace, gotBacklog := SplitDomain(tt.input)
			if gotSpace != tt.wantSpace {
				t.Errorf("space = %q, want %q", gotSpace, tt.wantSpace)
			}
			if gotBacklog != tt.wantBacklog {
				t.Errorf("backlog = %q, want %q", gotBacklog, tt.wantBacklog)
			}
		})
	}
}

func TestSplitAllowedDomain(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantSpace  string
		wantDomain string
		wantErr    bool
	}{
		{
			name:       "valid backlog.jp",
			input:      "myspace.backlog.jp",
			wantSpace:  "myspace",
			wantDomain: "backlog.jp",
			wantErr:    false,
		},
		{
			name:       "valid backlog.com",
			input:      "myspace.backlog.com",
			wantSpace:  "myspace",
			wantDomain: "backlog.com",
			wantErr:    false,
		},
		{
			name:    "single part",
			input:   "onlyspace",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "empty space",
			input:   ".backlog.jp",
			wantErr: true,
		},
		{
			name:    "empty domain",
			input:   "myspace.",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSpace, gotDomain, err := SplitAllowedDomain(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if gotSpace != tt.wantSpace {
				t.Errorf("space = %q, want %q", gotSpace, tt.wantSpace)
			}
			if gotDomain != tt.wantDomain {
				t.Errorf("domain = %q, want %q", gotDomain, tt.wantDomain)
			}
		})
	}
}

func TestNormalizeSpace(t *testing.T) {
	tests := []struct {
		name   string
		space  string
		domain string
		want   string
	}{
		{
			name:   "new format (already contains dot)",
			space:  "myspace.backlog.jp",
			domain: "",
			want:   "myspace.backlog.jp",
		},
		{
			name:   "old format (separate space and domain)",
			space:  "myspace",
			domain: "backlog.jp",
			want:   "myspace.backlog.jp",
		},
		{
			name:   "new format ignores domain",
			space:  "myspace.backlog.jp",
			domain: "backlog.com",
			want:   "myspace.backlog.jp",
		},
		{
			name:   "space only without domain",
			space:  "myspace",
			domain: "",
			want:   "myspace",
		},
		{
			name:   "both empty",
			space:  "",
			domain: "",
			want:   "",
		},
		{
			name:   "empty space with domain",
			space:  "",
			domain: "backlog.jp",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeSpace(tt.space, tt.domain)
			if got != tt.want {
				t.Errorf("NormalizeSpace(%q, %q) = %q, want %q", tt.space, tt.domain, got, tt.want)
			}
		})
	}
}

func TestSpaceID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"myspace.backlog.jp", "myspace"},
		{"myspace.backlog.com", "myspace"},
		{"myspace", "myspace"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := SpaceID(tt.input); got != tt.want {
				t.Errorf("SpaceID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSpaceDomain(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"myspace.backlog.jp", "backlog.jp"},
		{"myspace.backlog.com", "backlog.com"},
		{"myspace", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := SpaceDomain(tt.input); got != tt.want {
				t.Errorf("SpaceDomain(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
