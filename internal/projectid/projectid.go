package projectid

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FindGitRoot walks upward from startDir until it finds a .git directory
// or a .git file (linked worktrees and submodules), returning that
// directory's absolute path.
func FindGitRoot(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}

	for {
		gitPath := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitPath); err == nil && (info.IsDir() || info.Mode().IsRegular()) {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no .git directory or file found above %s", startDir)
		}
		dir = parent
	}
}

// CommonDir returns the absolute git common directory for a checkout.
// A normal repo and each of its linked worktrees share one common dir
// (the main .git). A submodule's common dir is its own git dir under
// the parent's modules path, so it stays a separate project.
func CommonDir(repoRoot string) (string, error) {
	gitPath := filepath.Join(repoRoot, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", err
	}
	gitDir := gitPath
	if !info.IsDir() {
		gitDir, err = parseGitdirFile(gitPath, repoRoot)
		if err != nil {
			return "", err
		}
	}

	//nolint:gosec // gitDir is this repo's own git directory, found by walking up from cwd
	data, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		if os.IsNotExist(err) {
			return gitDir, nil
		}
		return "", err
	}
	rel := strings.TrimSpace(string(data))
	if rel == "" || rel == "." {
		return gitDir, nil
	}
	if filepath.IsAbs(rel) {
		return filepath.Clean(rel), nil
	}
	abs, err := filepath.Abs(filepath.Join(gitDir, rel))
	if err != nil {
		return "", err
	}
	return abs, nil
}

// MainCheckout returns the main working tree when commonDir is a
// checkout's .git directory (a normal repo or a linked worktree).
// Submodule git dirs live under .git/modules and return ok=false.
func MainCheckout(commonDir string) (string, bool) {
	if filepath.Base(commonDir) != ".git" {
		return "", false
	}
	return filepath.Dir(commonDir), true
}

func parseGitdirFile(gitPath, repoRoot string) (string, error) {
	//nolint:gosec // gitPath is this checkout's .git file, found by walking up from cwd
	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
	const prefix = "gitdir:"
	if !strings.HasPrefix(line, prefix) {
		return "", fmt.Errorf("%s is not a gitdir file", gitPath)
	}
	dir := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if dir == "" {
		return "", fmt.Errorf("%s has an empty gitdir", gitPath)
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(repoRoot, dir)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("gitdir %s is not a directory", abs)
	}
	return abs, nil
}

// Derive returns a stable, filesystem-safe identifier for a path.
// Callers pass the git common dir so worktrees share one project id.
func Derive(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:8])
}
