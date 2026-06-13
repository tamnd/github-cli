package cli

import (
	"github.com/spf13/cobra"
	"github.com/tamnd/github-cli/github"
)

func (a *App) searchCmd() *cobra.Command {
	var (
		lang string
		sort string
	)
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search GitHub repositories",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n := a.effectiveLimit(20)
			opts := github.SearchRepoOptions{
				Query:    args[0],
				Language: lang,
				Sort:     sort,
				Limit:    n,
			}
			a.progressf("searching repositories for %q...", args[0])
			repos, err := a.client.SearchRepos(cmd.Context(), opts)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.renderOrEmpty(repos, len(repos))
		},
	}
	cmd.Flags().StringVar(&lang, "lang", "", "language filter (e.g. Go, Python)")
	cmd.Flags().StringVar(&sort, "sort", "stars", "sort order: stars|forks|updated|help-wanted-issues")
	return cmd
}
