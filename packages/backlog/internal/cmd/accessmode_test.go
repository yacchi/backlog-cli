package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func newCmd(parent *cobra.Command, name string) *cobra.Command {
	c := &cobra.Command{Use: name}
	if parent != nil {
		parent.AddCommand(c)
	}
	return c
}

func TestCheckAccessMode_NotSet(t *testing.T) {
	t.Setenv("BACKLOG_ACCESS_MODE", "")
	root := newCmd(nil, "backlog")
	create := newCmd(root, "create")
	if err := checkAccessMode(create); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestCheckAccessMode_ReadOnly_AllowsReadSubcommands(t *testing.T) {
	t.Setenv("BACKLOG_ACCESS_MODE", "read-only")
	root := newCmd(nil, "backlog")
	issue := newCmd(root, "issue")

	for _, name := range []string{"list", "view", "show", "count", "status", "download"} {
		c := newCmd(issue, name)
		if err := checkAccessMode(c); err != nil {
			t.Errorf("subcommand %q should be allowed: %v", name, err)
		}
	}
}

func TestCheckAccessMode_ReadOnly_BlocksWriteSubcommands(t *testing.T) {
	t.Setenv("BACKLOG_ACCESS_MODE", "read-only")
	root := newCmd(nil, "backlog")
	issue := newCmd(root, "issue")

	for _, name := range []string{"create", "edit", "delete", "comment", "close", "reopen"} {
		c := newCmd(issue, name)
		if err := checkAccessMode(c); err == nil {
			t.Errorf("subcommand %q should be blocked in read-only mode", name)
		}
	}
}

func TestCheckAccessMode_ReadOnly_AllowsReadOnlyTopLevel(t *testing.T) {
	t.Setenv("BACKLOG_ACCESS_MODE", "read-only")
	root := newCmd(nil, "backlog")

	for _, name := range []string{"whoami", "priority", "resolution", "profile"} {
		c := newCmd(root, name)
		if err := checkAccessMode(c); err != nil {
			t.Errorf("top-level %q should be allowed: %v", name, err)
		}
	}
}

func TestCheckAccessMode_ReadOnly_ApiCommand(t *testing.T) {
	t.Setenv("BACKLOG_ACCESS_MODE", "read-only")
	root := newCmd(nil, "backlog")

	apiGet := newCmd(root, "api")
	apiGet.Flags().StringP("method", "X", "GET", "")
	if err := checkAccessMode(apiGet); err != nil {
		t.Errorf("api GET should be allowed: %v", err)
	}

	root2 := newCmd(nil, "backlog")
	apiPost := newCmd(root2, "api")
	apiPost.Flags().StringP("method", "X", "GET", "")
	_ = apiPost.Flags().Set("method", "POST")
	if err := checkAccessMode(apiPost); err == nil {
		t.Error("api POST should be blocked in read-only mode")
	}
}
