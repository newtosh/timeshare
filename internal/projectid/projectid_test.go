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
