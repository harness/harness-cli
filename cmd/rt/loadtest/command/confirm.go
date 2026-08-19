package command

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// confirm asks the user to approve a destructive action. With no terminal to
// answer, the caller is told to pass --force rather than silently allowed.
func confirm(cmd *cobra.Command, prompt string) (bool, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return false, fmt.Errorf("%s\nstdin is not a terminal: pass --force to confirm non-interactively", prompt)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%s [y/N]: ", prompt)

	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("reading confirmation: %w", err)
	}

	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, nil
	}
	return false, nil
}
