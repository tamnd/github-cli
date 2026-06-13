package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) userCmd() *cobra.Command {
	var repos bool
	cmd := &cobra.Command{
		Use:   "user <username>",
		Short: "Show a GitHub user profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			username := args[0]
			a.progressf("fetching user %q...", username)
			user, err := a.client.GetUser(cmd.Context(), username)
			if err != nil {
				return mapFetchErr(err)
			}
			if err := a.render(user); err != nil {
				return err
			}
			if repos {
				n := a.effectiveLimit(10)
				a.progressf("fetching top %d repos for %q...", n, username)
				repoList, err := a.client.UserRepos(cmd.Context(), username, n)
				if err != nil {
					return mapFetchErr(err)
				}
				return a.render(repoList)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&repos, "repos", false, "also list the user's top repos")
	return cmd
}
