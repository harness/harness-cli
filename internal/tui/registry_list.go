package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	ar "github.com/harness/harness-cli/internal/api/ar"
	"github.com/harness/harness-cli/internal/style"
	client2 "github.com/harness/harness-cli/util/client"
)

// ─── Messages ────────────────────────────────────────────────────────────────

type registriesLoadedMsg struct {
	rows      []table.Row
	pageIndex int64
	pageCount int64
	itemCount int64
}

type registryErrorMsg struct{ err error }

// ─── Model ───────────────────────────────────────────────────────────────────

// RegistryListModel displays registries in an interactive table with
// pagination, filtering, and the ability to drill down.
type RegistryListModel struct {
	factory   *cmdutils.Factory
	table     table.Model
	spinner   spinner.Model
	loading   bool
	err       error
	pageIndex int64
	pageCount int64
	itemCount int64
	pageSize  int64
	quitting  bool
	width     int
	height    int
}

// NewRegistryListModel creates a new interactive registry list model.
func NewRegistryListModel(f *cmdutils.Factory) RegistryListModel {
	columns := []table.Column{
		{Title: "Registry", Width: 22},
		{Title: "Package Type", Width: 14},
		{Title: "Size", Width: 12},
		{Title: "Type", Width: 14},
		{Title: "Description", Width: 30},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(12),
	)

	// Style the table
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(style.Subtle).
		BorderBottom(true).
		Bold(true).
		Foreground(style.Cyan)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(style.Subtle).
		Bold(true)
	s.Cell = s.Cell.
		Foreground(lipgloss.Color("#FAFAFA"))
	t.SetStyles(s)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(style.SpinnerColor)

	return RegistryListModel{
		factory:  f,
		table:    t,
		spinner:  sp,
		loading:  true,
		pageSize: 15,
	}
}

func (m RegistryListModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchRegistries())
}

func (m RegistryListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.table.SetHeight(msg.Height - 8)
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c", "esc"))):
			m.quitting = true
			return m, tea.Quit
		case key.Matches(msg, key.NewBinding(key.WithKeys("n"))):
			// Next page
			if m.pageIndex < m.pageCount-1 {
				m.pageIndex++
				m.loading = true
				return m, tea.Batch(m.spinner.Tick, m.fetchRegistries())
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("p"))):
			// Prev page
			if m.pageIndex > 0 {
				m.pageIndex--
				m.loading = true
				return m, tea.Batch(m.spinner.Tick, m.fetchRegistries())
			}
		}

	case registriesLoadedMsg:
		m.loading = false
		m.pageIndex = msg.pageIndex
		m.pageCount = msg.pageCount
		m.itemCount = msg.itemCount
		m.table.SetRows(msg.rows)
		return m, nil

	case registryErrorMsg:
		m.loading = false
		m.err = msg.err
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	if !m.loading {
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m RegistryListModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	title := style.Title.Render("Artifact Registries")
	b.WriteString(title + "\n\n")

	if m.err != nil {
		b.WriteString(style.Error.Render("Error: "+m.err.Error()) + "\n")
		b.WriteString(style.Hint("Press q to go back"))
		return b.String()
	}

	if m.loading {
		b.WriteString(m.spinner.View() + " Loading registries...\n")
		return b.String()
	}

	b.WriteString(m.table.View())
	b.WriteString("\n\n")

	// Pagination info
	pageInfo := fmt.Sprintf("Page %d of %d (Total: %d)",
		m.pageIndex+1, m.pageCount, m.itemCount)
	b.WriteString(style.DimText.Render(pageInfo))
	b.WriteString("\n")

	// Help
	help := "↑/↓ navigate • n next page • p prev page • q quit"
	b.WriteString(style.StatusBar.Render(help))

	return b.String()
}

// fetchRegistries returns a tea.Cmd that loads registries from the API.
func (m RegistryListModel) fetchRegistries() tea.Cmd {
	return func() tea.Msg {
		params := &ar.GetAllRegistriesParams{}
		size := m.pageSize
		params.Size = &size
		if m.pageIndex > 0 {
			page := m.pageIndex
			params.Page = &page
		}

		response, err := m.factory.RegistryHttpClient().GetAllRegistriesWithResponse(
			context.Background(),
			client2.GetRef(config.Global.AccountID, config.Global.OrgID, config.Global.ProjectID),
			params,
		)
		if err != nil {
			return registryErrorMsg{err: err}
		}
		if response.JSON200 == nil || response.JSON200.Data.Registries == nil {
			return registryErrorMsg{err: fmt.Errorf("unexpected API response")}
		}

		data := response.JSON200.Data
		var rows []table.Row
		for _, reg := range data.Registries {
			identifier := reg.Identifier
			pkgType := string(reg.PackageType)
			regSize := ptrOrDash(reg.RegistrySize)
			regType := string(reg.Type)
			desc := ptrOrDash(reg.Description)
			rows = append(rows, table.Row{identifier, pkgType, regSize, regType, desc})
		}

		var pageIdx, pageCnt, itemCnt int64
		if data.PageIndex != nil {
			pageIdx = *data.PageIndex
		}
		if data.PageCount != nil {
			pageCnt = *data.PageCount
		}
		if data.ItemCount != nil {
			itemCnt = *data.ItemCount
		}

		return registriesLoadedMsg{
			rows:      rows,
			pageIndex: pageIdx,
			pageCount: pageCnt,
			itemCount: itemCnt,
		}
	}
}

// ptrOrDash safely dereferences a *string, returning "-" for nil/empty.
func ptrOrDash(v *string) string {
	if v == nil || *v == "" {
		return "-"
	}
	return *v
}

// RunRegistryList starts the interactive registry list TUI.
func RunRegistryList(f *cmdutils.Factory) error {
	m := NewRegistryListModel(f)
	p := tea.NewProgram(m, tea.WithAltScreen())

	_, err := p.Run()
	return err
}
