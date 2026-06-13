package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func (a *App) repoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "repo <owner/repo>",
		Short: "Show a single repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			owner, repo, err := splitOwnerRepo(args[0])
			if err != nil {
				return codeError(exitUsage, err)
			}
			a.progressf("fetching repo %s/%s...", owner, repo)
			r, err := a.client.GetRepo(cmd.Context(), owner, repo)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.render(r)
		},
	}
}

func splitOwnerRepo(s string) (owner, repo string, err error) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("argument must be owner/repo, got %q", s)
	}
	return parts[0], parts[1], nil
}
