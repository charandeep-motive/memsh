package ui

import (
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type pickerModel struct {
	title       string
	help        string
	input       textinput.Model
	allCommands []string
	filtered    []string
	cursor      int
	selected    string
	width       int
	height      int
	quitting    bool
	cancelled   bool
}

func RunCommandPicker(title string, commands []string, initialQuery string, output io.Writer) (string, error) {
	input := textinput.New()
	input.Placeholder = "Search commands"
	input.SetValue(initialQuery)
	input.Focus()
	input.CharLimit = 0
	input.Width = 50

	model := pickerModel{
		title:       title,
		help:        "Type to filter, Up/Down to move, Enter to select, Esc to cancel",
		input:       input,
		allCommands: commands,
	}
	model.filtered = model.filterCommands()

	program := tea.NewProgram(model, tea.WithOutput(output))
	finalModel, err := program.Run()
	if err != nil {
		return "", err
	}

	result := finalModel.(pickerModel)
	if result.cancelled {
		return "", nil
	}

	return result.selected, nil
}

func (m pickerModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			m.selected = m.filtered[m.cursor]
			m.quitting = true
			return m, tea.Quit
		case "up", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down", "ctrl+n":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.filtered = m.filterCommands()
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
	return m, cmd
}

func (m pickerModel) View() string {
	if m.quitting {
		return ""
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("110"))
	promptStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("111"))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	// sectionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("109"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Background(lipgloss.Color("238")).Bold(true)
	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	emptyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true)
	borderStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8")).Padding(1, 2)

	availableWidth := max(60, m.width-6)
	if configuredWidth := pickerWidth(); configuredWidth > 0 {
		availableWidth = min(configuredWidth, availableWidth)
	}
	if availableWidth > 140 {
		availableWidth = 140
	}
	separator := strings.Repeat("─", max(10, availableWidth-4))
	inputLine := promptStyle.Render("memsh> ") + m.input.View()

	lines := []string{titleStyle.Render(m.title), "", inputLine, helpStyle.Render(separator), ""}

	visibleItems := m.visibleCommands()
	if len(visibleItems) == 0 {
		lines = append(lines, emptyStyle.Render("No matching commands"))
	} else {
		for _, item := range visibleItems {
			if item.index == m.cursor {
				lines = append(lines, selectedStyle.Render("  "+item.command))
			} else {
				lines = append(lines, normalStyle.Render("  "+item.command))
			}
		}
	}

	lines = append(lines, "", helpStyle.Render(m.help))
	content := borderStyle.Width(availableWidth).Render(strings.Join(lines, "\n"))
	return lipgloss.NewStyle().Width(max(availableWidth, lipgloss.Width(content))).Render(content)
}

func (m pickerModel) filterCommands() []string {
	query := strings.ToLower(strings.TrimSpace(m.input.Value()))
	if query == "" {
		return append([]string(nil), m.allCommands...)
	}

	filtered := []string{}
	for _, command := range m.allCommands {
		if strings.Contains(strings.ToLower(command), query) {
			filtered = append(filtered, command)
		}
	}
	return filtered
}

type visibleCommand struct {
	index   int
	command string
}

func (m pickerModel) visibleCommands() []visibleCommand {
	if len(m.filtered) == 0 {
		return nil
	}

	maxItems := pickerMaxItems()
	if m.height > 12 {
		maxItems = min(12, m.height-8)
	}

	start := max(0, m.cursor-(maxItems/2))
	end := min(len(m.filtered), start+maxItems)
	if end-start < maxItems {
		start = max(0, end-maxItems)
	}

	items := make([]visibleCommand, 0, end-start)
	for index := start; index < end; index++ {
		items = append(items, visibleCommand{index: index, command: m.filtered[index]})
	}
	return items
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func pickerWidth() int {
	value := strings.TrimSpace(os.Getenv("MEMSH_UI_WIDTH"))
	if value == "" {
		return 100
	}

	width, err := strconv.Atoi(value)
	if err != nil || width < 60 {
		return 100
	}

	return width
}

func pickerMaxItems() int {
	value := strings.TrimSpace(os.Getenv("MEMSH_UI_MAX_ITEMS"))
	if value == "" {
		return 10
	}

	items, err := strconv.Atoi(value)
	if err != nil || items < 3 {
		return 10
	}

	return items
}
