package config_test

import (
	"testing"

	"github.com/charandeep-motive/memsh/internal/config"
)

func TestSuggestionLimit(t *testing.T) {
	t.Setenv("MEMSH_MAX_SUGGESTIONS", "9")
	if got := config.SuggestionLimit(); got != 9 {
		t.Fatalf("SuggestionLimit() = %d, want 9", got)
	}

	t.Setenv("MEMSH_MAX_SUGGESTIONS", "0")
	if got := config.SuggestionLimit(); got != 5 {
		t.Fatalf("SuggestionLimit() with zero = %d, want 5", got)
	}

	t.Setenv("MEMSH_MAX_SUGGESTIONS", "bad")
	if got := config.SuggestionLimit(); got != 5 {
		t.Fatalf("SuggestionLimit() with invalid env = %d, want 5", got)
	}

	t.Setenv("MEMSH_MAX_SUGGESTIONS", "")
	if got := config.SuggestionLimit(); got != 5 {
		t.Fatalf("SuggestionLimit() with empty env = %d, want default 5", got)
	}
}

func TestPickerWidthFallsBackBelowMinimum(t *testing.T) {
	t.Setenv("MEMSH_UI_WIDTH", "120")
	if got := config.PickerWidth(); got != 120 {
		t.Fatalf("PickerWidth() = %d, want 120", got)
	}

	t.Setenv("MEMSH_UI_WIDTH", "50")
	if got := config.PickerWidth(); got != 100 {
		t.Fatalf("PickerWidth() below minimum = %d, want default 100", got)
	}
}

func TestPickerMaxItemsFallsBackBelowMinimum(t *testing.T) {
	t.Setenv("MEMSH_UI_MAX_ITEMS", "12")
	if got := config.PickerMaxItems(); got != 12 {
		t.Fatalf("PickerMaxItems() = %d, want 12", got)
	}

	t.Setenv("MEMSH_UI_MAX_ITEMS", "2")
	if got := config.PickerMaxItems(); got != 10 {
		t.Fatalf("PickerMaxItems() below minimum = %d, want default 10", got)
	}
}

func TestDirectoryAwarenessEnabled(t *testing.T) {
	t.Setenv("MEMSH_ENABLE_DIRECTORY_AWARENESS", "1")
	if !config.DirectoryAwarenessEnabled() {
		t.Fatal("DirectoryAwarenessEnabled() = false, want true when set to 1")
	}

	t.Setenv("MEMSH_ENABLE_DIRECTORY_AWARENESS", "0")
	if config.DirectoryAwarenessEnabled() {
		t.Fatal("DirectoryAwarenessEnabled() = true, want false when set to 0")
	}

	t.Setenv("MEMSH_ENABLE_DIRECTORY_AWARENESS", "")
	if config.DirectoryAwarenessEnabled() {
		t.Fatal("DirectoryAwarenessEnabled() = true, want false when unset")
	}
}

func TestSaveLogsEnabledDefaultsOff(t *testing.T) {
	t.Setenv("MEMSH_SAVE_LOGS", "")
	if config.SaveLogsEnabled() {
		t.Error("SaveLogsEnabled() = true with empty env, want false")
	}
}

func TestSaveLogsEnabledWhenSet(t *testing.T) {
	t.Setenv("MEMSH_SAVE_LOGS", "1")
	if !config.SaveLogsEnabled() {
		t.Error("SaveLogsEnabled() = false with MEMSH_SAVE_LOGS=1, want true")
	}
}

func TestLogRetentionDaysDefault(t *testing.T) {
	t.Setenv("MEMSH_LOG_RETENTION_DAYS", "")
	if got := config.LogRetentionDays(); got != 10 {
		t.Errorf("LogRetentionDays() = %d, want 10", got)
	}
}

func TestLogRetentionDaysCustom(t *testing.T) {
	t.Setenv("MEMSH_LOG_RETENTION_DAYS", "30")
	if got := config.LogRetentionDays(); got != 30 {
		t.Errorf("LogRetentionDays() = %d, want 30", got)
	}
}
