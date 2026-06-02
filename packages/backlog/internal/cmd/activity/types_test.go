package activity

import (
	"reflect"
	"testing"
)

func TestParseTypes(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []int
		wantErr bool
	}{
		{"semantic names", "issue-create,issue-update,issue-comment", []int{1, 2, 3}, false},
		{"numeric ids", "1,14", []int{1, 14}, false},
		{"mixed with spaces", " issue-create , 3 ", []int{1, 3}, false},
		{"dedup", "issue-create,issue-create,1", []int{1}, false},
		{"unknown name", "issue-create,bogus", nil, true},
		{"out of range numeric", "99", nil, true},
		{"empty", "", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTypes(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParseTypes(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestTypeName(t *testing.T) {
	if got := TypeName(3); got != "issue-comment" {
		t.Fatalf("TypeName(3) = %q, want issue-comment", got)
	}
	if got := TypeName(14); got != "issue-bulk-update" {
		t.Fatalf("TypeName(14) = %q, want issue-bulk-update", got)
	}
	if got := TypeName(999); got != "type-999" {
		t.Fatalf("TypeName(999) = %q, want type-999", got)
	}
}
