package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/charandeep-motive/memsh/internal/config"
)

func runSettings(args []string, output io.Writer) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	stored, err := config.ReadSettingsFile(paths.SettingsPath)
	if err != nil {
		return err
	}

	if len(args) == 0 || args[0] == "list" {
		return printSettings(output, paths.SettingsPath, stored)
	}

	switch args[0] {
	case "path":
		_, err := fmt.Fprintln(output, paths.SettingsPath)
		return err
	case "set":
		if len(args) < 3 {
			return errors.New("usage: memsh settings set KEY VALUE")
		}
		key := strings.TrimSpace(args[1])
		value := strings.TrimSpace(strings.Join(args[2:], " "))
		if err := config.ValidateSetting(key, value); err != nil {
			return err
		}
		stored[key] = value
		if err := config.WriteSettingsFile(paths.SettingsPath, stored); err != nil {
			return err
		}
		_, err := fmt.Fprintf(output, "saved %s=%s\nreload shell: source ~/.zshrc\n", key, value)
		return err
	case "unset":
		if len(args) != 2 {
			return errors.New("usage: memsh settings unset KEY")
		}
		key := strings.TrimSpace(args[1])
		if _, ok := config.LookupSetting(key); !ok {
			return fmt.Errorf("unsupported setting %q", key)
		}
		delete(stored, key)
		if err := config.WriteSettingsFile(paths.SettingsPath, stored); err != nil {
			return err
		}
		_, err := fmt.Fprintf(output, "removed %s\nreload shell: source ~/.zshrc\n", key)
		return err
	default:
		return fmt.Errorf("unknown settings command %q", args[0])
	}
}

func printSettings(output io.Writer, settingsPath string, stored map[string]string) error {
	defaults := config.DefaultSettings()
	if _, err := fmt.Fprintf(output, "settings_file=%s\n", settingsPath); err != nil {
		return err
	}

	for _, spec := range config.SupportedSettings() {
		value, ok := stored[spec.Key]
		source := "default"
		if ok {
			source = "saved"
		} else {
			value = defaults[spec.Key]
		}

		if _, err := fmt.Fprintf(output, "%s=%s [%s]\n", spec.Key, value, source); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(output, "  %s (%s)\n", spec.Description, spec.ValueHint); err != nil {
			return err
		}
	}

	_, err := fmt.Fprintln(output, "\ncommands: memsh settings set KEY VALUE | memsh settings unset KEY | memsh settings path")
	return err
}
