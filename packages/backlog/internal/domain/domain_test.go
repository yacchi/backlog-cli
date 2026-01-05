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
