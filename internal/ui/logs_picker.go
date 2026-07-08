package ui

import (
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/charandeep-motive/memsh/internal/config"
	"github.com/charandeep-motive/memsh/internal/db"
)

// Terminal escape / control sequences that a `script` recording captures. They
// must be stripped before recorded output is shown in the picker or the pager:
// left in place they move the cursor, switch screen buffers, or emit OSC colour
// queries (vim and other full-screen programs are common sources) that corrupt
// the surrounding UI.
var (
	// oscSequence matches OSC sequences (ESC ] … terminated by BEL, ST, the next
	// ESC, or end of line), including the colour queries vim emits.
	oscSequence = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)?`)
	// stringCommand matches DCS/SOS/PM/APC strings (ESC P/X/^/_ … ST).
	stringCommand = regexp.MustCompile(`\x1b[PX^_][^\x1b]*\x1b\\`)
	// csiSequence matches any CSI sequence (ESC [ params intermediates final).
	csiSequence = regexp.MustCompile(`\x1b\[[\x30-\x3f]*[\x20-\x2f]*[\x40-\x7e]`)
	// csiNonSGR matches CSI sequences other than SGR colour/style (final != 'm').
	csiNonSGR = regexp.MustCompile(`\x1b\[[\x30-\x3f]*[\x20-\x2f]*[\x40-\x6c\x6e-\x7e]`)
	// escTwoByte matches a two-byte escape whose second byte is not '[', so SGR
	// (ESC [ … m) is left untouched; used only on the colour-preserving path.
	escTwoByte = regexp.MustCompile(`\x1b[^[]`)
	// c0Control matches C0 control characters except tab and newline.
	c0Control = regexp.MustCompile(`[\x00-\x08\x0b-\x1f\x7f]`)
	// c0ControlKeepEsc is c0Control but keeps ESC (0x1b) so SGR sequences survive.
	c0ControlKeepEsc = regexp.MustCompile(`[\x00-\x08\x0b-\x1a\x1c-\x1f\x7f]`)
	// promptMarker matches a zsh PROMPT_SP end-of-line marker line — a lone "%"
	// padded with spaces — that a recording captures just before a prompt.
	promptMarker = regexp.MustCompile(`^%\s*$`)
)

// sanitizeForDisplay strips terminal escape sequences and control characters so
// recorded output can be shown safely. When keepColor is true, SGR colour/style
// sequences are preserved (for a colour-capable pager); otherwise everything is
// removed, leaving plain text.
func sanitizeForDisplay(s string, keepColor bool) string {
	s = oscSequence.ReplaceAllString(s, "")
	s = stringCommand.ReplaceAllString(s, "")
	if keepColor {
		s = csiNonSGR.ReplaceAllString(s, "")
		s = escTwoByte.ReplaceAllString(s, "")
		return c0ControlKeepEsc.ReplaceAllString(s, "")
	}
	s = csiSequence.ReplaceAllString(s, "")
	s = escTwoByte.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\x1b", "")
	return c0Control.ReplaceAllString(s, "")
}

// flattenCarriageReturns applies terminal carriage-return semantics to a single
// line: it drops a trailing \r (from a \r\n ending), then keeps only the text
// after any final mid-line \r (a terminal rewrites the line from column zero
// after each bare \r, so progress bars and spinners collapse to their final
// state).
func flattenCarriageReturns(line string) string {
	line = strings.TrimSuffix(line, "\r")
	if idx := strings.LastIndexByte(line, '\r'); idx >= 0 {
		line = line[idx+1:]
	}
	return line
}

func isScriptHeader(line string) bool {
	return strings.HasPrefix(line, "Script started on ") || strings.HasPrefix(line, "Script done on ")
}

// RenderForPager flattens a raw `script` recording for display in a pager
// (e.g. `less -R`): it drops BSD script header/footer lines, applies
// carriage-return overwrites, and strips cursor/screen-control and OSC
// sequences while preserving SGR colours so the full view stays colourised.
func RenderForPager(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if isScriptHeader(line) {
			continue
		}
		line = flattenCarriageReturns(line)
		line = sanitizeForDisplay(line, true)
		out = append(out, line)
	}
	return []byte(strings.Join(out, "\n"))
}

// cleanTerminalOutput flattens a raw `script` recording into plain display
// lines. It drops BSD script header/footer lines, applies carriage-return
// overwrites, strips all escape sequences and control characters, and removes
// blank and zsh prompt-marker lines.
func cleanTerminalOutput(data []byte) []string {
	rawLines := strings.Split(string(data), "\n")
	cleaned := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		if isScriptHeader(line) {
			continue
		}

		line = flattenCarriageReturns(line)
		line = sanitizeForDisplay(line, false)

		if promptMarker.MatchString(line) {
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		cleaned = append(cleaned, line)
	}
	return cleaned
}

type logsPickerModel struct {
	title     string
	help      string
	input     textinput.Model
	allLogs   []db.CommandLog
	filtered  []db.CommandLog
	cursor    int
	selected  string
	preview   []string
	width     int
	height    int
	quitting  bool
	cancelled bool
}

// RunLogsPicker opens the interactive logs picker and returns the selected log file path.
// Returns "" if the user cancels or nothing is selected.
func RunLogsPicker(title string, logs []db.CommandLog, output io.Writer) (string, error) {
	input := textinput.New()
	input.Placeholder = "Search commands"
	input.Focus()
	input.CharLimit = 0
	input.Width = 50

	model := logsPickerModel{
		title:   title,
		help:    "Type to filter, Up/Down to move, Enter to view log, Esc to cancel",
		input:   input,
		allLogs: logs,
	}
	model.filtered = model.filterLogs()
	model.preview = model.loadPreview()

	// Use the alternate screen: this picker is tall (list + preview pane), and
	// the inline renderer garbles frames taller than the terminal window
	// (duplicated header/input lines). The alt screen clears each frame and is
	// restored on exit — the standard behaviour for a full-screen picker.
	program := tea.NewProgram(model, tea.WithOutput(output), tea.WithAltScreen())
	finalModel, err := program.Run()
	if err != nil {
		return "", err
	}

	result := finalModel.(logsPickerModel)
	if result.cancelled {
		return "", nil
	}
	return result.selected, nil
}

func (m logsPickerModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m logsPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.width > 0 {
			m.input.Width = min(60, m.width-10)
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			m.quitting = true
			return m, tea.Quit
		case "enter":
			if len(m.filtered) == 0 {
				return m, nil
			}
			m.selected = m.filtered[m.cursor].LogFile
			m.quitting = true
			return m, tea.Quit
		case "up", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
				m.preview = m.loadPreview()
			}
			return m, nil
		case "down", "ctrl+n":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
				m.preview = m.loadPreview()
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.filtered = m.filterLogs()
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
	m.preview = m.loadPreview()
	return m, cmd
}

func (m logsPickerModel) View() string {
	if m.quitting {
		return ""
	}

	titleStyle    := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("110"))
	promptStyle   := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("111"))
	helpStyle     := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Background(lipgloss.Color("238")).Bold(true)
	normalStyle   := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	emptyStyle    := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true)
	previewStyle  := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	borderStyle   := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8")).Padding(1, 2)

	availableWidth := max(60, m.width-6)
	if configuredWidth := config.PickerWidth(); configuredWidth > 0 {
		availableWidth = min(configuredWidth, availableWidth)
	}
	if availableWidth > 140 {
		availableWidth = 140
	}
	separator := strings.Repeat("─", max(10, availableWidth-4))
	inputLine := promptStyle.Render("memsh> ") + m.input.View()

	lines := []string{titleStyle.Render(m.title), "", inputLine, helpStyle.Render(separator), ""}

	rowWidth := max(10, availableWidth-2-4-2)

	// Command list — up to 5 items in logs picker to leave room for preview.
	maxItems := 5
	if m.height > 20 {
		maxItems = 7
	}

	visible := m.visibleLogs(maxItems)
	if len(visible) == 0 {
		lines = append(lines, emptyStyle.Render("No matching logs"))
	} else {
		for _, v := range visible {
			ts := v.log.ExecutedAt.Format("Jan _2 15:04")
			display := ts + "  " + v.log.Command
			if v.index == m.cursor {
				lines = append(lines, selectedStyle.Render("  "+display))
			} else {
				lines = append(lines, normalStyle.Render("  "+truncate(display, rowWidth)))
			}
		}
	}

	// Preview section.
	lines = append(lines, "", helpStyle.Render("── preview "+strings.Repeat("─", max(5, availableWidth-14))))
	if len(m.preview) == 0 {
		lines = append(lines, emptyStyle.Render("  [no output captured]"))
	} else {
		for _, l := range m.preview {
			lines = append(lines, previewStyle.Render("  "+truncate(l, rowWidth)))
		}
	}

	lines = append(lines, "", helpStyle.Render(m.help))
	content := borderStyle.Width(availableWidth).Render(strings.Join(lines, "\n"))
	return lipgloss.NewStyle().Width(max(availableWidth, lipgloss.Width(content))).Render(content)
}

type visibleLog struct {
	index int
	log   db.CommandLog
}

func (m logsPickerModel) visibleLogs(maxItems int) []visibleLog {
	if len(m.filtered) == 0 {
		return nil
	}

	start := max(0, m.cursor-(maxItems/2))
	end := min(len(m.filtered), start+maxItems)
	if end-start < maxItems {
		start = max(0, end-maxItems)
	}

	items := make([]visibleLog, 0, end-start)
	for i := start; i < end; i++ {
		items = append(items, visibleLog{index: i, log: m.filtered[i]})
	}
	return items
}

func (m logsPickerModel) filterLogs() []db.CommandLog {
	query := strings.ToLower(strings.TrimSpace(m.input.Value()))
	if query == "" {
		return append([]db.CommandLog(nil), m.allLogs...)
	}
	filtered := []db.CommandLog{}
	for _, l := range m.allLogs {
		if strings.Contains(strings.ToLower(l.Command), query) {
			filtered = append(filtered, l)
		}
	}
	return filtered
}

// loadPreview reads up to 8 lines from the focused log file, stripping BSD
// script headers and ANSI escape sequences. Returns nil if nothing to show.
func (m logsPickerModel) loadPreview() []string {
	if len(m.filtered) == 0 {
		return nil
	}
	cl := m.filtered[m.cursor]

	data, err := os.ReadFile(cl.LogFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{"[log expired]"}
		}
		return nil
	}
	if len(data) == 0 {
		return []string{"[no output captured]"}
	}

	cleaned := cleanTerminalOutput(data)

	// Show last up to 8 lines.
	const maxPreviewLines = 8
	if len(cleaned) > maxPreviewLines {
		cleaned = cleaned[len(cleaned)-maxPreviewLines:]
	}
	return cleaned
}
