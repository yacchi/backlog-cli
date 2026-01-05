package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

var hashCost int

var hashCmd = &cobra.Command{
	Use:   "hash [passphrase]",
	Short: "Generate bcrypt hash for portal passphrase",
	Long: `Generate a bcrypt hash for use as passphrase_hash in tenant configuration.

If passphrase is not provided as argument, it will be read from stdin (hidden input).

Examples:
  backlog config hash mysecret
  backlog config hash                  # prompts for passphrase
  echo "mysecret" | backlog config hash`,
	Args: cobra.MaximumNArgs(1),
	RunE: runHash,
}

func init() {
	hashCmd.Flags().IntVar(&hashCost, "cost", 12, "bcrypt cost factor (10-14 recommended)")
}

func runHash(cmd *cobra.Command, args []string) error {
	var passphrase string

	if len(args) == 1 {
		passphrase = args[0]
	} else {
		// Check if stdin is a terminal
		if term.IsTerminal(int(syscall.Stdin)) {
			fmt.Fprint(os.Stderr, "Enter passphrase: ")
			bytePassphrase, err := term.ReadPassword(int(syscall.Stdin))
			if err != nil {
				return fmt.Errorf("failed to read passphrase: %w", err)
			}
			fmt.Fprintln(os.Stderr) // newline after hidden input
			passphrase = string(bytePassphrase)
		} else {
			// Read from pipe
			reader := bufio.NewReader(os.Stdin)
			line, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read passphrase: %w", err)
			}
			passphrase = strings.TrimSpace(line)
		}
	}

	if passphrase == "" {
		return fmt.Errorf("passphrase cannot be empty")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(passphrase), hashCost)
	if err != nil {
		return fmt.Errorf("failed to generate hash: %w", err)
	}

	fmt.Println(string(hash))
	return nil
}
