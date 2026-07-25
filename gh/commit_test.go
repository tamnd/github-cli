package gh

import "testing"

// The patch parser is the one part of the history layer that is pure text in
// and records out, so it is the one part that can be tested without the
// network. Everything it gets wrong, it gets wrong silently, which is exactly
// what these cases are for.

const twoCommitMailbox = `From 1111111111111111111111111111111111111111 Mon Sep 17 00:00:00 2001
From: Ada Lovelace <1234+ada@users.noreply.github.com>
Date: Thu, 7 Nov 2024 14:39:11 -0700
Subject: [PATCH 1/2] teach the engine to count

The analytical engine could not previously count past ten.
---
 engine.go | 3 ++-
 1 file changed, 2 insertions(+), 1 deletion(-)

diff --git a/engine.go b/engine.go
index aaa..bbb 100644
--- a/engine.go
+++ b/engine.go
@@ -1,3 +1,4 @@
 package engine
-const max = 10
+const max = 1000
+const min = 0
--
2.47.0

From 2222222222222222222222222222222222222222 Mon Sep 17 00:00:00 2001
From: Grace Hopper <grace@example.com>
Date: Fri, 8 Nov 2024 09:00:00 +0000
Subject: [PATCH 2/2] rename the moth file

---
 moth.txt => bug.txt | 0
 1 file changed, 0 insertions(+), 0 deletions(-)

diff --git a/moth.txt b/bug.txt
similarity index 100%
rename from moth.txt
rename to bug.txt
--
2.47.0
`

func TestSplitMailbox(t *testing.T) {
	parts := splitMailbox(twoCommitMailbox)
	if len(parts) != 2 {
		t.Fatalf("split into %d parts, want 2", len(parts))
	}
	if got := parts[0][:45]; got[:5] != "From " {
		t.Errorf("first part starts %q", got)
	}
}

func TestSplitMailboxIgnoresFromInsideADiff(t *testing.T) {
	// A changed file whose content begins with "From " is the trap this
	// parser exists to avoid. Splitting on "From " alone cuts this in half.
	const tricky = `From 1111111111111111111111111111111111111111 Mon Sep 17 00:00:00 2001
From: Ada <ada@example.com>
Subject: [PATCH] add a letter

---
diff --git a/letter.txt b/letter.txt
+From Ada, with regards
+From 2222222222222222222222222222222222222222 was not a header
`
	if n := len(splitMailbox(tricky)); n != 1 {
		t.Fatalf("split into %d parts, want 1", n)
	}
}

func TestCommitFromPatch(t *testing.T) {
	parts := splitMailbox(twoCommitMailbox)
	first := commitFromPatch("cli/cli", parts[0])
	if first == nil {
		t.Fatal("first commit did not parse")
	}
	if first.SHA != "1111111111111111111111111111111111111111" {
		t.Errorf("sha %q", first.SHA)
	}
	// The [PATCH 1/2] prefix is git's, not the author's, and it has no place
	// on a record.
	if first.Subject != "teach the engine to count" {
		t.Errorf("subject %q", first.Subject)
	}
	if first.Body != "The analytical engine could not previously count past ten." {
		t.Errorf("body %q", first.Body)
	}
	if len(first.Authors) != 1 {
		t.Fatalf("authors %+v", first.Authors)
	}
	a := first.Authors[0]
	if a.Name != "Ada Lovelace" {
		t.Errorf("author name %q", a.Name)
	}
	// The noreply address is the only place a patch carries a GitHub login.
	if a.Login != "ada" {
		t.Errorf("author login %q, the noreply address should have given it", a.Login)
	}
	if first.AuthoredAt == nil || first.AuthoredAt.UTC().Format("2006-01-02T15:04:05Z") != "2024-11-07T21:39:11Z" {
		t.Errorf("authored at %v", first.AuthoredAt)
	}
	if first.ID != "cli/cli@1111111111111111111111111111111111111111" {
		t.Errorf("id %q", first.ID)
	}

	second := commitFromPatch("cli/cli", parts[1])
	if second == nil {
		t.Fatal("second commit did not parse")
	}
	// A plain address has no login and the record should say so rather than
	// invent one from the local part.
	if len(second.Authors) != 1 || second.Authors[0].Login != "" {
		t.Errorf("second authors %+v", second.Authors)
	}
	if second.Authors[0].Name != "Grace Hopper" {
		t.Errorf("second author name %q", second.Authors[0].Name)
	}
}

func TestFilesInPatch(t *testing.T) {
	files := filesInPatch(twoCommitMailbox)
	if len(files) != 2 {
		t.Fatalf("files %+v", files)
	}
	e := files[0]
	if e.Path != "engine.go" || e.Status != "modified" {
		t.Errorf("first file %+v", e)
	}
	// Two + lines and one - line. Neither file header counts, and neither
	// does the "--" that ends the mailbox entry.
	if e.Additions == nil || *e.Additions != 2 {
		t.Errorf("additions %v, want 2", e.Additions)
	}
	if e.Deletions == nil || *e.Deletions != 1 {
		t.Errorf("deletions %v, want 1", e.Deletions)
	}
	r := files[1]
	if r.Status != "renamed" || r.Path != "bug.txt" || r.PrevPath != "moth.txt" {
		t.Errorf("rename %+v", r)
	}
}

func TestLoginFromNoreply(t *testing.T) {
	cases := []struct {
		email string
		want  string
	}{
		{"1234+octocat@users.noreply.github.com", "octocat"},
		{"octocat@users.noreply.github.com", "octocat"},
		{"octocat@example.com", ""},
		{"", ""},
		{"@users.noreply.github.com", ""},
	}
	for _, c := range cases {
		got, ok := loginFromNoreply(c.email)
		if !ok {
			got = ""
		}
		if got != c.want {
			t.Errorf("%q gave %q, want %q", c.email, got, c.want)
		}
	}
}

func TestLooksLikeSize(t *testing.T) {
	yes := []string{"13.1 MB", "742 Bytes", "1 KB", "2.5 GB"}
	no := []string{"", "Latest", "13.1MB", "sha256:abc", "v2.96.0", "MB 13.1"}
	for _, s := range yes {
		if !looksLikeSize(s) {
			t.Errorf("%q should read as a size", s)
		}
	}
	for _, s := range no {
		if looksLikeSize(s) {
			t.Errorf("%q should not read as a size", s)
		}
	}
}

func TestPathFromDiffHeader(t *testing.T) {
	cases := map[string]string{
		"diff --git a/engine.go b/engine.go":         "engine.go",
		"diff --git a/moth.txt b/bug.txt":            "bug.txt",
		"diff --git a/docs/api.md b/docs/api.md":     "docs/api.md",
		"diff --git a/x.go b/pkg/nested/b/deep/y.go": "pkg/nested/b/deep/y.go",
	}
	for line, want := range cases {
		if got := pathFromDiffHeader(line); got != want {
			t.Errorf("%q gave %q, want %q", line, got, want)
		}
	}
}

func TestFilesInPatchPrefersTheHeaderPath(t *testing.T) {
	// The diff --git line cannot be split when a path contains a space, so
	// the +++ header is the authority and has to win.
	const spaced = `diff --git a/my file b/my file
index aaa..bbb 100644
--- a/my file
+++ b/my file
@@ -1 +1 @@
-old
+new
`
	files := filesInPatch(spaced)
	if len(files) != 1 {
		t.Fatalf("files %+v", files)
	}
	if files[0].Path != "my file" {
		t.Errorf("path %q, the +++ header should have corrected the guess", files[0].Path)
	}
}
