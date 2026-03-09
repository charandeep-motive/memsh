package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Paths struct {
	DataDir      string
	DatabasePath string
}

func ResolvePaths() (Paths, error) {
	dataRoot, err := resolveDataRoot()
	if err != nil {
		return Paths{}, err
	}

	dataDir := filepath.Join(dataRoot, "memsh")
	return Paths{
		DataDir:      dataDir,
		DatabasePath: filepath.Join(dataDir, "memsh.db"),
	}, nil
}

func resolveDataRoot() (string, error) {
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		return xdg, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}

	return filepath.Join(home, ".local", "share"), nil
}
