// Package tui provides the interactive Bubble Tea TUI for the Harness CLI.
// When the user runs `hc` with no arguments in a TTY (or passes --interactive),
// this package launches a main menu that lets them pick a command, fill in
// parameters via a form, and see styled output.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/harness/harness-cli/internal/style"
)

// ─── Menu item ───────────────────────────────────────────────────────────────

// menuItem represents a single entry in the main interactive menu.
type menuItem struct {
	title       string
	description string
	command     string // the cobra command path, e.g. "registry list"
}

func (i menuItem) Title() string       { return i.title }
func (i menuItem) Description() string { return i.description }
func (i menuItem) FilterValue() string { return i.title }

// ─── Key map ─────────────────────────────────────────────────────────────────

type menuKeyMap struct {
	Quit   key.Binding
	Enter  key.Binding
	Filter key.Binding
}

var menuKeys = menuKeyMap{
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q/ctrl+c", "quit"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	Filter: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "filter"),
	),
}

// ─── Model ───────────────────────────────────────────────────────────────────

// MenuModel is the top-level Bubble Tea model for the interactive main menu.
type MenuModel struct {
	list     list.Model
	choice   string // selected command path
	quitting bool
	width    int
	height   int
}

// SelectedCommand returns the cobra command path the user picked,
// or "" if they quit without choosing.
func (m MenuModel) SelectedCommand() string { return m.choice }

// NewMenuModel builds the interactive main menu with all available commands.
func NewMenuModel() MenuModel {
	items := []list.Item{
		menuItem{
			title:       "Login",
			description: "Authenticate with Harness services",
			command:     "auth login",
		},
		menuItem{
			title:       "Auth Status",
			description: "Check current authentication status",
			command:     "auth status",
		},
		menuItem{
			title:       "List Registries",
			description: "List all artifact registries",
			command:     "registry list",
		},
		menuItem{
			title:       "Get Registry",
			description: "Get details of a specific registry",
			command:     "registry get",
		},
		menuItem{
			title:       "Delete Registry",
			description: "Delete an artifact registry",
			command:     "registry delete",
		},
		menuItem{
			title:       "List Artifacts",
			description: "List all artifacts in a registry",
			command:     "artifact list",
		},
		menuItem{
			title:       "Get Artifact",
			description: "Get details of a specific artifact",
			command:     "artifact get",
		},
		menuItem{
			title:       "Push Artifact",
			description: "Push a package to an artifact registry",
			command:     "artifact push",
		},
		menuItem{
			title:       "Pull Artifact",
			description: "Pull a package from an artifact registry",
			command:     "artifact pull",
		},
		menuItem{
			title:       "Version",
			description: "Print CLI version information",
			command:     "version",
		},
		menuItem{
			title:       "Upgrade",
			description: "Upgrade CLI to the latest version",
			command:     "upgrade",
		},
		menuItem{
			title:       "Logout",
			description: "Remove saved credentials",
			command:     "auth logout",
		},
	}

	// Delegate for styled rendering
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(style.Cyan).
		BorderLeftForeground(style.Cyan)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(style.Dim).
		BorderLeftForeground(style.Cyan)
	delegate.Styles.NormalTitle = delegate.Styles.NormalTitle.
		Foreground(lipgloss.Color("#FAFAFA"))
	delegate.Styles.NormalDesc = delegate.Styles.NormalDesc.
		Foreground(style.Dim)

	l := list.New(items, delegate, 60, 20)
	l.Title = "Harness CLI — What would you like to do?"
	l.Styles.Title = style.MenuTitle
	l.SetFilteringEnabled(true)
	l.SetShowStatusBar(true)
	l.SetShowHelp(true)

	return MenuModel{list: l}
}

// ─── Bubble Tea interface ────────────────────────────────────────────────────

func (m MenuModel) Init() tea.Cmd {
	return nil
}

func (m MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height-2)
		return m, nil

	case tea.KeyMsg:
		// Don't intercept keys while filtering
		if m.list.FilterState() == list.Filtering {
			break
		}
		switch {
		case key.Matches(msg, menuKeys.Quit):
			m.quitting = true
			return m, tea.Quit
		case key.Matches(msg, menuKeys.Enter):
			if item, ok := m.list.SelectedItem().(menuItem); ok {
				m.choice = item.command
				return m, tea.Quit
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m MenuModel) View() string {
	if m.quitting {
		return style.DimText.Render("Goodbye!") + "\n"
	}
	if m.choice != "" {
		return "" // we'll run the command after the TUI exits
	}

	var b strings.Builder
	b.WriteString(m.list.View())
	b.WriteString("\n")
	b.WriteString(style.StatusBar.Render("↑/↓ navigate • / filter • enter select • q quit"))
	return b.String()
}

// ─── Runner ──────────────────────────────────────────────────────────────────

// RunMenu starts the interactive main menu and returns the selected command
// path (e.g. "registry list") or "" if the user quit.
func RunMenu() (string, error) {
	m := NewMenuModel()
	p := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("TUI error: %w", err)
	}

	result := finalModel.(MenuModel)
	return result.SelectedCommand(), nil
}
