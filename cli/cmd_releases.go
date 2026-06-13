package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) releasesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "releases <owner/repo>",
		Short: "List releases for a repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			owner, repo, err := splitOwnerRepo(args[0])
			if err != nil {
				return codeError(exitUsage, err)
			}
			n := a.effectiveLimit(20)
			a.progressf("fetching releases for %s/%s...", owner, repo)
			releases, err := a.client.Releases(cmd.Context(), owner, repo, n)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.renderOrEmpty(releases, len(releases))
		},
	}
}
