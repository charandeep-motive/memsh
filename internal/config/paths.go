package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Paths struct {
	ConfigDir    string
	SettingsPath string
	DataDir      string
	DatabasePath string
}

func ResolvePaths() (Paths, error) {
	configRoot, err := resolveConfigRoot()
	if err != nil {
		return Paths{}, err
	}

	dataRoot, err := resolveDataRoot()
	if err != nil {
		return Paths{}, err
	}

	configDir := filepath.Join(configRoot, "memsh")
	dataDir := filepath.Join(dataRoot, "memsh")
	return Paths{
		ConfigDir:    configDir,
		SettingsPath: filepath.Join(configDir, "settings.zsh"),
		DataDir:      dataDir,
		DatabasePath: filepath.Join(dataDir, "memsh.db"),
	}, nil
}

func resolveConfigRoot() (string, error) {
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return xdg, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}

	return filepath.Join(home, ".config"), nil
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
