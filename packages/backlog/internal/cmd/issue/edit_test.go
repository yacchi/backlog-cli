package issue

import (
	"strings"
	"testing"
)

// TestValidateEditParentFlags_AllowsNeither is the common case: no parent
// flag at all.
func TestValidateEditParentFlags_AllowsNeither(t *testing.T) {
	editParent = ""
	editRemoveParent = false
	defer func() {
		editParent = ""
		editRemoveParent = false
	}()

	if err := validateEditParentFlags(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestValidateEditParentFlags_AllowsParentAlone verifies --parent alone is fine.
func TestValidateEditParentFlags_AllowsParentAlone(t *testing.T) {
	editParent = "PROJ-1"
	editRemoveParent = false
	defer func() {
		editParent = ""
		editRemoveParent = false
	}()

	if err := validateEditParentFlags(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestValidateEditParentFlags_AllowsRemoveParentAlone verifies --remove-parent
// alone is fine.
func TestValidateEditParentFlags_AllowsRemoveParentAlone(t *testing.T) {
	editParent = ""
	editRemoveParent = true
	defer func() {
		editParent = ""
		editRemoveParent = false
	}()

	if err := validateEditParentFlags(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestValidateEditParentFlags_RejectsBoth is an adversarial test derived from
// T3's contract requirement: "--parent and --remove-parent together is a
// usage error: fail before any API call with a message naming both flags."
func TestValidateEditParentFlags_RejectsBoth(t *testing.T) {
	editParent = "PROJ-1"
	editRemoveParent = true
	defer func() {
		editParent = ""
		editRemoveParent = false
	}()

	err := validateEditParentFlags()
	if err == nil {
		t.Fatal("expected error when --parent and --remove-parent are combined")
	}
	if !strings.Contains(err.Error(), "--parent") || !strings.Contains(err.Error(), "--remove-parent") {
		t.Fatalf("error %q must name both --parent and --remove-parent", err.Error())
	}
}
