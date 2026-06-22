package issue

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func newViewTestCmd() (*cobra.Command, *string) {
	var comments string
	cmd := &cobra.Command{
		Use:  "view <issue-key>",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 2 {
				v := args[1]
				validComments := v == "all" || v == "0"
				if !validComments {
					_, err := strconv.Atoi(v)
					validComments = err == nil
				}
				if c.Flags().Changed("comments") && comments == "default" && validComments {
					comments = v
				} else {
					return fmt.Errorf("unexpected argument: %s", args[1])
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&comments, "comments", "c", "", "Show comments")
	cmd.Flags().Lookup("comments").NoOptDefVal = "default"
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	return cmd, &comments
}

func TestViewCmdCommentsAbsorption(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantComments   string
		wantErr        bool
		wantErrContain string
	}{
		{
			name:         "-c all (space-separated)",
			args:         []string{"PROJ-1", "-c", "all"},
			wantComments: "all",
		},
		{
			name:         "-c 50 (space-separated)",
			args:         []string{"PROJ-1", "-c", "50"},
			wantComments: "50",
		},
		{
			name:         "-c=all (equals-separated)",
			args:         []string{"PROJ-1", "-c=all"},
			wantComments: "all",
		},
		{
			name:         "-c without value (NoOptDefVal)",
			args:         []string{"PROJ-1", "-c"},
			wantComments: "default",
		},
		{
			name:           "extra arg without -c",
			args:           []string{"PROJ-1", "extra"},
			wantErr:        true,
			wantErrContain: "unexpected argument",
		},
		{
			name:           "-c before issue key: -c all PROJ-1 rejects non-numeric second arg",
			args:           []string{"-c", "all", "PROJ-1"},
			wantErr:        true,
			wantErrContain: "unexpected argument",
		},
		{
			name:           "-c before issue key: -c 50 PROJ-1 rejects non-numeric second arg",
			args:           []string{"-c", "50", "PROJ-1"},
			wantErr:        true,
			wantErrContain: "unexpected argument",
		},
		{
			name:         "-c all --comments-order asc (with other flags)",
			args:         []string{"PROJ-1", "-c", "all", "--comments-order", "asc"},
			wantComments: "all",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, comments := newViewTestCmd()
			cmd.Flags().String("comments-order", "desc", "order")
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrContain != "" && !strings.Contains(err.Error(), tt.wantErrContain) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantErrContain)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if *comments != tt.wantComments {
				t.Errorf("comments = %q, want %q", *comments, tt.wantComments)
			}
		})
	}
}
