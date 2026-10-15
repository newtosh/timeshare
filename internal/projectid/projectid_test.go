package projectid

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindGitRoot(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "repo")
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatal(err)
	}

	got, err := FindGitRoot(nested)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != root {
		t.Fatalf("got %q, want %q", got, root)
	}
}

func TestFindGitRootNoRepo(t *testing.T) {
	tmp := t.TempDir()
	_, err := FindGitRoot(tmp)
	if err == nil {
		t.Fatal("expected error when no .git directory exists")
	}
}

func TestFindGitRootAcceptsGitFile(t *testing.T) {
	tmp := t.TempDir()
	work := filepath.Join(tmp, "work")
	if err := os.MkdirAll(work, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, ".git"), []byte("gitdir: /nowhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := FindGitRoot(work)
	if err != nil {
		t.Fatal(err)
	}
	if got != work {
		t.Fatalf("got %q, want %q", got, work)
	}
}

func TestWorktreeSharesCommonDirWithMainCheckout(t *testing.T) {
	tmp := t.TempDir()
	main := filepath.Join(tmp, "main")
	wtGit := filepath.Join(main, ".git", "worktrees", "feature")
	if err := os.MkdirAll(wtGit, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtGit, "commondir"), []byte("../..\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(tmp, "feature")
	if err := os.MkdirAll(work, 0o750); err != nil {
		t.Fatal(err)
	}
	gitdir := "gitdir: " + wtGit + "\n"
	if err := os.WriteFile(filepath.Join(work, ".git"), []byte(gitdir), 0o600); err != nil {
		t.Fatal(err)
	}

	mainCommon, err := CommonDir(main)
	if err != nil {
		t.Fatal(err)
	}
	wtCommon, err := CommonDir(work)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(main, ".git")
	if mainCommon != want || wtCommon != want {
		t.Fatalf("common dirs main=%q worktree=%q want %q", mainCommon, wtCommon, want)
	}
	if Derive(mainCommon) != Derive(wtCommon) {
		t.Fatal("worktree and main checkout must share a project id")
	}
	checkout, ok := MainCheckout(wtCommon)
	if !ok || checkout != main {
		t.Fatalf("MainCheckout = %q ok=%v, want %q", checkout, ok, main)
	}
}

func TestSubmoduleCommonDirIsItsOwnGitDir(t *testing.T) {
	tmp := t.TempDir()
	parent := filepath.Join(tmp, "parent")
	mod := filepath.Join(parent, ".git", "modules", "sub")
	if err := os.MkdirAll(mod, 0o750); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(parent, "sub")
	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	rel := filepath.Join("..", ".git", "modules", "sub")
	if err := os.WriteFile(filepath.Join(sub, ".git"), []byte("gitdir: "+rel+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	common, err := CommonDir(sub)
	if err != nil {
		t.Fatal(err)
	}
	if common != mod {
		t.Fatalf("common = %q, want %q", common, mod)
	}
	if _, ok := MainCheckout(common); ok {
		t.Fatal("a submodule git dir must not be treated as the parent checkout")
	}
	parentCommon, err := CommonDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	if Derive(common) == Derive(parentCommon) {
		t.Fatal("submodule must not share the parent project id")
	}
}

func TestDeriveIsStableAndDistinct(t *testing.T) {
	a := Derive("/home/user/repo-a")
	b := Derive("/home/user/repo-a")
	c := Derive("/home/user/repo-b")

	if a != b {
		t.Fatalf("Derive must be deterministic: %q != %q", a, b)
	}
	if a == c {
		t.Fatal("different repo paths must produce different project IDs")
	}
	if len(a) != 16 {
		t.Fatalf("expected 16-char hex id, got %d chars: %q", len(a), a)
	}
}
