package tui

import (
	"fmt"
	"time"

	"github.com/76creates/stickers/flexbox"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// LiveView shows recent pipeline runs with auto-refresh
type LiveView struct {
	logsRoot string
	width    int
	height   int

	runs        []RunSummary
	flexBox     *flexbox.FlexBox
	lastRefresh time.Time
}

// NewLiveView creates a new live monitoring view
func NewLiveView(logsRoot string) *LiveView {
	l := &LiveView{
		logsRoot: logsRoot,
	}
	l.initFlexBox()
	return l
}

// initFlexBox initializes the flexbox layout
func (l *LiveView) initFlexBox() {
	l.flexBox = flexbox.New(0, 0).SetStyle(
		lipgloss.NewStyle().Background(lipgloss.Color("#1a1a1a")),
	)
}

// Init initializes the view
func (l *LiveView) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (l *LiveView) Update(msg tea.Msg) (*LiveView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		l.SetSize(msg.Width, msg.Height)
	}
	return l, nil
}

// View renders the live view
func (l *LiveView) View() string {
	if l.width == 0 || l.height == 0 {
		return "Loading..."
	}

	recentRuns := GetRecentRuns(l.runs, 60*time.Minute)
	recentStats := CalculateStats(recentRuns)

	l.flexBox.SetWidth(l.width)
	l.flexBox.SetHeight(l.height)

	row1 := l.flexBox.NewRow().AddCells(
		flexbox.NewCell(1, 1).SetStyle(
			lipgloss.NewStyle().
				Foreground(lipgloss.Color("#7d8b96")).
				Bold(true).
				Align(lipgloss.Left).
				Padding(1, 0),
		).SetContent("Live Monitoring"),
	)

	row2 := l.flexBox.NewRow().AddCells(
		flexbox.NewCell(1, 1).SetStyle(
			lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6a7a8c")).
				Align(lipgloss.Left).
				Padding(0, 0),
		).SetContent("Showing runs from last 60 minutes"),
	)

	var statsContent string
	if recentStats.TotalRuns == 0 {
		statsContent = "No activity in the last hour"
	} else {
		statsContent = fmt.Sprintf(
			"Recent: %d runs | Success: %d | Failed: %d",
			recentStats.TotalRuns,
			recentStats.SuccessCount,
			recentStats.FailCount,
		)
	}

	row3 := l.flexBox.NewRow().AddCells(
		flexbox.NewCell(1, 1).SetStyle(
			lipgloss.NewStyle().
				Foreground(lipgloss.Color("#8a9aa6")).
				Align(lipgloss.Left).
				Padding(1, 0),
		).SetContent(statsContent),
	)

	var runsContent string
	if len(recentRuns) == 0 {
		runsContent = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#5a7a8c")).
			Render("No recent runs to display")
	} else {
		var rows []string
		for _, run := range recentRuns {
			row := lipgloss.JoinHorizontal(lipgloss.Left,
				GetStatusIcon(run.Status), " ",
				truncateString(run.RunID, 25), "  ",
				lipgloss.NewStyle().Foreground(lipgloss.Color("#5a7a8c")).Render(
					FormatDurationHuman(time.Since(run.CompletedAt))+" ago",
				),
			)
			rows = append(rows, row)
		}
		runsContent = lipgloss.JoinVertical(lipgloss.Left, rows...)
	}

	row4 := l.flexBox.NewRow().AddCells(
		flexbox.NewCell(1, 8).SetStyle(
			lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#4a4a4a")).
				Padding(1, 1),
		).SetContent(lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.NewStyle().Foreground(lipgloss.Color("#6a9a7a")).Bold(true).Render("Recent Runs"),
			"",
			runsContent,
		)),
	)

	l.flexBox.SetRows([]*flexbox.Row{row1, row2, row3, row4})
	l.flexBox.ForceRecalculate()

	return l.flexBox.Render()
}

// SetSize updates the view size
func (l *LiveView) SetSize(width, height int) {
	l.width = width
	l.height = height
}

// SetRuns updates the runs data
func (l *LiveView) SetRuns(runs []RunSummary) {
	l.runs = runs
	l.lastRefresh = time.Now()
}
