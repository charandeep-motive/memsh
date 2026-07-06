package config

import (
	"os"
	"strconv"
	"strings"
)

// This file centralizes reading runtime settings from the environment so the
// default values declared in settingSpecs are the single source of truth.
// Callers use these typed accessors instead of reading os.Getenv directly.

// SuggestionLimit returns the maximum number of suggestions to return.
func SuggestionLimit() int {
	return positiveIntSetting("MEMSH_MAX_SUGGESTIONS", 1)
}

// PickerWidth returns the preferred interactive picker width in columns.
func PickerWidth() int {
	return positiveIntSetting("MEMSH_UI_WIDTH", 60)
}

// PickerMaxItems returns the maximum number of visible items in the picker.
func PickerMaxItems() int {
	return positiveIntSetting("MEMSH_UI_MAX_ITEMS", 3)
}

// DirectoryAwarenessEnabled reports whether commands used in the current
// directory should be ranked ahead of global suggestions.
func DirectoryAwarenessEnabled() bool {
	return strings.TrimSpace(os.Getenv("MEMSH_ENABLE_DIRECTORY_AWARENESS")) == "1"
}

// positiveIntSetting reads an integer setting from the environment, falling
// back to the setting's declared default when the value is unset, invalid,
// or below the given minimum.
func positiveIntSetting(key string, minimum int) int {
	fallback := 0
	if spec, ok := LookupSetting(key); ok {
		if parsed, err := strconv.Atoi(spec.Default); err == nil {
			fallback = parsed
		}
	}

	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minimum {
		return fallback
	}

	return parsed
}
