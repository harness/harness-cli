package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/harness/harness-cli/internal/style"
)

// ─── Messages ────────────────────────────────────────────────────────────────

// SpinnerDoneMsg signals that the long-running operation finished.
type SpinnerDoneMsg struct {
	Result interface{}
	Err    error
}

// ─── Model ───────────────────────────────────────────────────────────────────

// SpinnerModel shows a spinner while a background operation runs.
type SpinnerModel struct {
	spinner  spinner.Model
	title    string
	done     bool
	err      error
	result   interface{}
	runFunc  func() (interface{}, error)
	quitting bool
}

// NewSpinnerModel creates a spinner that runs fn in the background.
func NewSpinnerModel(title string, fn func() (interface{}, error)) SpinnerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(style.SpinnerColor)

	return SpinnerModel{
		spinner: s,
		title:   title,
		runFunc: fn,
	}
}

func (m SpinnerModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			result, err := m.runFunc()
			return SpinnerDoneMsg{Result: result, Err: err}
		},
	)
}

func (m SpinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}

	case SpinnerDoneMsg:
		m.done = true
		m.err = msg.Err
		m.result = msg.Result
		return m, tea.Quit

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m SpinnerModel) View() string {
	if m.quitting {
		return ""
	}
	if m.done {
		if m.err != nil {
			return style.Error.Render(fmt.Sprintf("✗ %s: %v", m.title, m.err)) + "\n"
		}
		return style.Success.Render(fmt.Sprintf("✓ %s", m.title)) + "\n"
	}
	return m.spinner.View() + " " + m.title + "...\n"
}

// RunWithSpinner runs fn while showing a spinner. Returns the result or error.
func RunWithSpinner(title string, fn func() (interface{}, error)) (interface{}, error) {
	m := NewSpinnerModel(title, fn)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	result := finalModel.(SpinnerModel)
	if result.err != nil {
		return nil, result.err
	}
	return result.result, nil
}
