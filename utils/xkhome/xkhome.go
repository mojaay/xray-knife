// Package xkhome resolves where xray-knife keeps its state: the database,
// system-proxy backups, and netns bookkeeping. Every caller goes through here
// so XRAY_KNIFE_HOME relocates all of it at once.
package xkhome

import (
	"os"
	"path/filepath"
)

// Dir returns the xray-knife state directory, creating it if missing.
// XRAY_KNIFE_HOME wins if set; otherwise ~/.xray-knife.
func Dir() (string, error) {
	dir := os.Getenv("XRAY_KNIFE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".xray-knife")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// DBPath returns the SQLite database path. A non-empty override (the --db
// flag) is used verbatim; otherwise the database lives under Dir().
func DBPath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "xray-knife.db"), nil
}
