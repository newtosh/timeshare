package cli

import (
	"os"

	"github.com/newtosh/timeshare/internal/config"
)

// loadExistingConfig reports whether cfgPath exists and, if so, attempts to
// parse it. exists is true whenever the file is present, even if parsing
// fails — the caller needs to distinguish "no file" (blank wizard) from
// "file present but broken" (still a real error to surface).
func loadExistingConfig(cfgPath string) (config.Config, bool, error) {
	if _, statErr := os.Stat(cfgPath); statErr != nil {
		if os.IsNotExist(statErr) {
			return config.Config{}, false, nil
		}
		return config.Config{}, false, statErr
	}

	cfg, err := config.Load(cfgPath)
	return cfg, true, err
}
