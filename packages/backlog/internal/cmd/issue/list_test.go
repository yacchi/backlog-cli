package issue

import (
	"reflect"
	"testing"
)

func TestParseProjectScope(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantKeys  []string
		wantCross bool
		wantErr   bool
	}{
		{
			name:      "single project",
			raw:       "INFRA",
			wantKeys:  []string{"INFRA"},
			wantCross: false,
		},
		{
			name:      "multiple projects",
			raw:       "INFRA,LCS",
			wantKeys:  []string{"INFRA", "LCS"},
			wantCross: false,
		},
		{
			name:      "multiple projects with spaces",
			raw:       " INFRA , LCS ",
			wantKeys:  []string{"INFRA", "LCS"},
			wantCross: false,
		},
		{
			name:      "cross-project keyword",
			raw:       "all",
			wantKeys:  nil,
			wantCross: true,
		},
		{
			name:      "uppercase ALL is a real project key, not the keyword",
			raw:       "ALL",
			wantKeys:  []string{"ALL"},
			wantCross: false,
		},
		{
			name:    "empty value is an error",
			raw:     "",
			wantErr: true,
		},
		{
			name:    "only commas is an error",
			raw:     " , ",
			wantErr: true,
		},
		{
			name:    "cross-project keyword cannot be combined with keys",
			raw:     "all,INFRA",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys, cross, err := parseProjectScope(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got keys=%v cross=%v", keys, cross)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cross != tt.wantCross {
				t.Errorf("crossProject = %v, want %v", cross, tt.wantCross)
			}
			if !reflect.DeepEqual(keys, tt.wantKeys) {
				t.Errorf("keys = %v, want %v", keys, tt.wantKeys)
			}
		})
	}
}
