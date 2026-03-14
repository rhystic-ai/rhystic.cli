package tui

import (
	"fmt"
	"time"

	"github.com/76creates/stickers/flexbox"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TabType represents the different tabs
type TabType int

const (
	TabDashboard TabType = iota
	TabLive
)

// App is the main TUI application
type App struct {
	logsRoot string
	width    int
	height   int

	theme Theme

	// Views
	dashboard *DashboardView
	liveView  *LiveView

	// Layout
	flexBox *flexbox.FlexBox
	tabRow  *flexbox.Row

	// State
	activeTab  TabType
	err        error
	loading    bool
	loadingMsg string
}

// NewApp creates a new TUI application
func NewApp(logsRoot string) *App {
	if logsRoot == "" {
		logsRoot = DefaultLogsRoot
	}

	app := &App{
		logsRoot:   logsRoot,
		theme:      NewTheme(),
		loading:    true,
		loadingMsg: "Loading pipeline data...",
	}

	app.dashboard = NewDashboardView(logsRoot)
	app.liveView = NewLiveView(logsRoot)
	app.initFlexBox()

	return app
}

// initFlexBox initializes the main layout
func (a *App) initFlexBox() {
	a.flexBox = flexbox.New(0, 0).SetStyle(
		lipgloss.NewStyle().Background(ColorBackground),
	)
}

// Init initializes the application
func (a *App) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		tea.EnableMouseCellMotion,
		a.loadData(),
		tea.Tick(time.Second*3, func(t time.Time) tea.Msg {
			return tickMsg(t)
		}),
	)
}

// Update handles messages
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.flexBox.SetWidth(msg.Width)
		a.flexBox.SetHeight(msg.Height)
		a.dashboard.SetSize(msg.Width, a.height-4)
		a.liveView.SetSize(msg.Width, a.height-4)

	case tea.KeyMsg:
		if !a.dashboard.showDetail {
			switch msg.String() {
			case "q", "ctrl+c":
				return a, tea.Quit
			case "tab", "right":
				a.nextTab()
			case "shift+tab", "left":
				a.prevTab()
			case "1":
				a.activeTab = TabDashboard
			case "2":
				a.activeTab = TabLive
			case "r":
				cmds = append(cmds, a.loadData())
			}
		} else {
			switch msg.String() {
			case "q":
				a.dashboard.showDetail = false
				a.dashboard.selected = nil
				return a, nil
			}
		}

	case dataLoadedMsg:
		a.loading = false
		if msg.err != nil {
			a.err = msg.err
		}
		a.dashboard.runs = msg.runs
		a.dashboard.stats = CalculateStats(msg.runs)
		a.dashboard.updateTableRows()
		a.liveView.SetRuns(msg.runs)

	case tickMsg:
		if a.activeTab == TabLive {
			cmds = append(cmds, a.loadData())
		}
		cmds = append(cmds, tea.Tick(time.Second*3, func(t time.Time) tea.Msg {
			return tickMsg(t)
		}))

	case error:
		a.err = msg
		a.loading = false
	}

	if !a.loading && a.err == nil {
		if a.activeTab == TabDashboard {
			newDashboard, cmd := a.dashboard.Update(msg)
			a.dashboard = newDashboard
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		} else {
			newLive, cmd := a.liveView.Update(msg)
			a.liveView = newLive
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}

	return a, tea.Batch(cmds...)
}

// View renders the application
func (a *App) View() string {
	if a.width == 0 || a.height == 0 {
		return "Initializing..."
	}

	if a.loading {
		return a.renderLoading()
	}

	if a.err != nil {
		return a.renderError()
	}

	return a.renderMain()
}

// renderMain renders the main layout using FlexBox
func (a *App) renderMain() string {
	a.flexBox.SetWidth(a.width)
	a.flexBox.SetHeight(a.height)

	var rows []*flexbox.Row

	if !a.dashboard.showDetail {
		headerRow := a.flexBox.NewRow().AddCells(
			flexbox.NewCell(1, 1).SetStyle(
				lipgloss.NewStyle().
					Background(lipgloss.Color("#252525")).
					Foreground(lipgloss.Color("#7d8b96")).
					Bold(true).
					Padding(0, 1).
					Align(lipgloss.Left),
			).SetContent("⚡ ATTRACTOR DASHBOARD"),
		)
		rows = append(rows, headerRow)

		tabRow := a.renderTabRow()
		rows = append(rows, tabRow)
	}

	contentHeight := a.height - 3
	if a.dashboard.showDetail {
		contentHeight = a.height - 1
	}

	var content string
	if a.activeTab == TabDashboard {
		a.dashboard.SetSize(a.width, contentHeight)
		content = a.dashboard.View()
	} else {
		a.liveView.SetSize(a.width, contentHeight)
		content = a.liveView.View()
	}

	contentRow := a.flexBox.NewRow().AddCells(
		flexbox.NewCell(1, contentHeight).SetContent(content),
	)
	rows = append(rows, contentRow)

	footerRow := a.flexBox.NewRow().AddCells(
		flexbox.NewCell(1, 1).SetStyle(
			lipgloss.NewStyle().
				Background(lipgloss.Color("#252525")).
				Foreground(lipgloss.Color("#6a7a8c")).
				Padding(0, 1),
		).SetContent(a.renderHelp()),
	)
	rows = append(rows, footerRow)

	a.flexBox.SetRows(rows)
	a.flexBox.ForceRecalculate()

	return a.flexBox.Render()
}

// renderTabRow renders the tab navigation
func (a *App) renderTabRow() *flexbox.Row {
	dashboardStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#252525")).
		Foreground(lipgloss.Color("#6a6a6a")).
		Padding(0, 2)
	liveStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#252525")).
		Foreground(lipgloss.Color("#6a6a6a")).
		Padding(0, 2)

	if a.activeTab == TabDashboard {
		dashboardStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#5a7a8c")).
			Foreground(lipgloss.Color("#1a1a1a")).
			Bold(true).
			Padding(0, 2)
	} else {
		liveStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#5a7a8c")).
			Foreground(lipgloss.Color("#1a1a1a")).
			Bold(true).
			Padding(0, 2)
	}

	return a.flexBox.NewRow().AddCells(
		flexbox.NewCell(1, 1).SetStyle(dashboardStyle).SetContent("1: Dashboard"),
		flexbox.NewCell(1, 1).SetStyle(liveStyle).SetContent("2: Live"),
	)
}

// renderHelp renders the help text
func (a *App) renderHelp() string {
	if a.dashboard.showDetail {
		return "ESC: back • ↑/↓: navigate • Ctrl+S: sort • q: quit"
	}
	return "↑/↓: navigate • Enter: details • Tab/1/2: tabs • Ctrl+S: sort • r: refresh • q: quit"
}

// renderLoading renders the loading state
func (a *App) renderLoading() string {
	a.flexBox.SetWidth(a.width)
	a.flexBox.SetHeight(a.height)

	row := a.flexBox.NewRow().AddCells(
		flexbox.NewCell(1, 1).SetStyle(
			lipgloss.NewStyle().
				Foreground(lipgloss.Color("#8a9aa6")).
				Bold(true).
				Align(lipgloss.Center),
		).SetContent(a.loadingMsg),
	)
	a.flexBox.SetRows([]*flexbox.Row{row})

	return a.flexBox.Render()
}

// renderError renders an error state
func (a *App) renderError() string {
	a.flexBox.SetWidth(a.width)
	a.flexBox.SetHeight(a.height)

	row := a.flexBox.NewRow().AddCells(
		flexbox.NewCell(1, 1).SetStyle(
			lipgloss.NewStyle().
				Foreground(ColorError).
				Bold(true).
				Align(lipgloss.Center),
		).SetContent(fmt.Sprintf("Error: %v", a.err)),
	)
	a.flexBox.SetRows([]*flexbox.Row{row})

	return a.flexBox.Render()
}

// nextTab switches to the next tab
func (a *App) nextTab() {
	a.activeTab = TabType((int(a.activeTab) + 1) % 2)
}

// prevTab switches to the previous tab
func (a *App) prevTab() {
	a.activeTab = TabType((int(a.activeTab) - 1 + 2) % 2)
}

// Message types
type dataLoadedMsg struct {
	runs []RunSummary
	err  error
}

type tickMsg time.Time

// loadData loads the run data asynchronously
func (a *App) loadData() tea.Cmd {
	return func() tea.Msg {
		runs, err := LoadRuns(a.logsRoot)
		return dataLoadedMsg{runs: runs, err: err}
	}
}

// Run starts the TUI application
func Run(logsRoot string) error {
	app := NewApp(logsRoot)
	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseAllMotion())
	_, err := p.Run()
	return err
}
