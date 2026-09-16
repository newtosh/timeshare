package projectid

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// FindGitRoot walks upward from startDir until it finds a directory
// containing .git, returning that directory's absolute path.
func FindGitRoot(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}

	for {
		if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil && info.IsDir() {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no .git directory found above %s", startDir)
		}
		dir = parent
	}
}

// Derive returns a stable, filesystem-safe identifier for a repo root path.
func Derive(repoRoot string) string {
	sum := sha256.Sum256([]byte(repoRoot))
	return hex.EncodeToString(sum[:8])
}
