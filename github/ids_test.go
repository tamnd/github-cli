package github

import "testing"

func TestParseRepoSlug(t *testing.T) {
	cases := []struct {
		in        string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{"golang/go", "golang", "go", false},
		{"torvalds/linux", "torvalds", "linux", false},
		{"owner/repo-name", "owner", "repo-name", false},
		{"", "", "", true},
		{"noslash", "", "", true},
		{"too/many/slashes", "", "", true},
		{"/noleadingslash", "", "", true},
		{"trailingslash/", "", "", true},
	}
	for _, tc := range cases {
		owner, repo, err := ParseRepoSlug(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseRepoSlug(%q): want error, got owner=%q repo=%q", tc.in, owner, repo)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseRepoSlug(%q): unexpected error: %v", tc.in, err)
			continue
		}
		if owner != tc.wantOwner {
			t.Errorf("ParseRepoSlug(%q): owner want %q, got %q", tc.in, tc.wantOwner, owner)
		}
		if repo != tc.wantRepo {
			t.Errorf("ParseRepoSlug(%q): repo want %q, got %q", tc.in, tc.wantRepo, repo)
		}
	}
}
