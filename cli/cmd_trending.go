package cli

import (
	"github.com/spf13/cobra"
	"github.com/tamnd/github-cli/github"
)

func (a *App) trendingCmd() *cobra.Command {
	var (
		lang string
		days int
	)
	cmd := &cobra.Command{
		Use:   "trending",
		Short: "Show trending repositories (proxy via search API)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			n := a.effectiveLimit(25)
			opts := github.TrendingOptions{
				Language: lang,
				Days:     days,
				Limit:    n,
			}
			a.progressf("fetching trending repositories (last %d days)...", days)
			repos, err := a.client.Trending(cmd.Context(), opts)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.renderOrEmpty(repos, len(repos))
		},
	}
	cmd.Flags().StringVar(&lang, "lang", "", "language filter (e.g. Go, Python)")
	cmd.Flags().IntVar(&days, "days", 7, "time window in days (7, 30, or 365)")
	return cmd
}
