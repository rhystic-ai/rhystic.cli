package tui

import (
	"fmt"
	"time"

	"github.com/76creates/stickers/flexbox"
	"github.com/76creates/stickers/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// DashboardView shows the main dashboard with stats and run table
type DashboardView struct {
	logsRoot string
	theme    Theme
	width    int
	height   int

	runs     []RunSummary
	stats    DashboardStats
	table    *table.Table
	statsBox *flexbox.FlexBox
	selected *RunDetail

	showDetail bool
}

// NewDashboardView creates a new dashboard
func NewDashboardView(logsRoot string) *DashboardView {
	headers := []string{"Run ID", "Pipeline", "Status", "Nodes", "Time"}
	t := table.NewTable(0, 0, headers)

	t.SetRatio([]int{3, 2, 2, 1, 1})
	t.SetMinWidth([]int{22, 15, 10, 6, 8})
	t.SetStyles(GetTableStyles())
	t.SetStylePassing(true)

	return &DashboardView{
		logsRoot: logsRoot,
		theme:    NewTheme(),
		table:    t,
		statsBox: flexbox.New(0, 0),
	}
}

// Init initializes the view
func (d *DashboardView) Init() tea.Cmd {
	return d.loadData()
}

// Update handles messages
func (d *DashboardView) Update(msg tea.Msg) (*DashboardView, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.SetSize(msg.Width, msg.Height)

	case tea.KeyMsg:
		if d.showDetail {
			switch msg.String() {
			case "esc", "backspace":
				d.showDetail = false
				d.selected = nil
				return d, nil
			}
		} else {
			switch msg.String() {
			case "enter":
				x, y := d.table.GetCursorLocation()
				if y >= 0 && y < len(d.runs) {
					_ = x
					cmds = append(cmds, d.loadRunDetail(d.runs[y].RunID))
				}
			case "r":
				cmds = append(cmds, d.loadData())
			case "up", "k":
				d.table.CursorUp()
			case "down", "j":
				d.table.CursorDown()
			case "left", "h":
				d.table.CursorLeft()
			case "right", "l":
				d.table.CursorRight()
			case "ctrl+s":
				x, _ := d.table.GetCursorLocation()
				_, order := d.table.GetOrder()
				if order == table.SortingOrderAscending {
					d.table.OrderByDesc(x)
				} else {
					d.table.OrderByAsc(x)
				}
			}
		}

	case dashboardRunsLoadedMsg:
		d.runs = msg.runs
		d.stats = CalculateStats(msg.runs)
		d.updateTableRows()

	case dashboardRunDetailLoadedMsg:
		d.selected = msg.detail
		d.showDetail = true
	}

	return d, tea.Batch(cmds...)
}

// View renders the dashboard
func (d *DashboardView) View() string {
	if d.showDetail && d.selected != nil {
		return d.renderPipelineDetail()
	}
	return d.renderDashboard()
}

// renderDashboard shows the main dashboard
func (d *DashboardView) renderDashboard() string {
	if d.width == 0 || d.height == 0 {
		return "Loading..."
	}

	var sections []string

	title := d.theme.HeaderTitle.Render("⚡ ATTRACTOR DASHBOARD")
	subtitle := d.theme.Base.Foreground(ColorMuted).Render(fmt.Sprintf("%d runs total", d.stats.TotalRuns))
	header := lipgloss.NewStyle().Width(d.width).Render(
		lipgloss.JoinHorizontal(lipgloss.Left, title, "  ", subtitle),
	)
	sections = append(sections, header)

	cards := d.renderSummaryCards()
	sections = append(sections, "", cards)

	sections = append(sections, "", d.theme.PanelTitle.Render("Pipeline Runs"))
	sections = append(sections, d.table.Render())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderSummaryCards shows statistics cards using flexbox
func (d *DashboardView) renderSummaryCards() string {
	if d.stats.TotalRuns == 0 {
		return d.theme.Base.Foreground(lipgloss.Color("#5a5a5a")).Render("No runs available")
	}

	d.statsBox.SetWidth(d.width)
	d.statsBox.SetHeight(5)

	row := d.statsBox.NewRow()

	totalCell := flexbox.NewCell(1, 1).
		SetStyle(lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#5a7a8c")).
			Align(lipgloss.Center).
			Padding(0, 1)).
		SetContent(lipgloss.JoinVertical(lipgloss.Center,
			lipgloss.NewStyle().Foreground(lipgloss.Color("#8a9aa6")).Bold(true).Render(fmt.Sprintf("%d", d.stats.TotalRuns)),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#d4d4d4")).Render("Total"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#5a5a5a")).Render("runs"),
		))

	successCell := flexbox.NewCell(1, 1).
		SetStyle(lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#5a8a6a")).
			Align(lipgloss.Center).
			Padding(0, 1)).
		SetContent(lipgloss.JoinVertical(lipgloss.Center,
			lipgloss.NewStyle().Foreground(lipgloss.Color("#6a9a7a")).Bold(true).Render(fmt.Sprintf("%d", d.stats.SuccessCount)),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#d4d4d4")).Render("Success"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#5a5a5a")).Render(FormatRate(d.stats.SuccessRate)),
		))

	failCell := flexbox.NewCell(1, 1).
		SetStyle(lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#8a5a5a")).
			Align(lipgloss.Center).
			Padding(0, 1)).
		SetContent(lipgloss.JoinVertical(lipgloss.Center,
			lipgloss.NewStyle().Foreground(lipgloss.Color("#9a6a6a")).Bold(true).Render(fmt.Sprintf("%d", d.stats.FailCount)),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#d4d4d4")).Render("Failed"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#5a5a5a")).Render(FormatRate(d.stats.FailRate)),
		))

	pendingCell := flexbox.NewCell(1, 1).
		SetStyle(lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#5a7a8c")).
			Align(lipgloss.Center).
			Padding(0, 1)).
		SetContent(lipgloss.JoinVertical(lipgloss.Center,
			lipgloss.NewStyle().Foreground(lipgloss.Color("#6a7a8c")).Bold(true).Render(fmt.Sprintf("%d", d.stats.PendingCount)),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#d4d4d4")).Render("Pending"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#5a5a5a")).Render("waiting"),
		))

	nodesCell := flexbox.NewCell(1, 1).
		SetStyle(lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7a8a9a")).
			Align(lipgloss.Center).
			Padding(0, 1)).
		SetContent(lipgloss.JoinVertical(lipgloss.Center,
			lipgloss.NewStyle().Foreground(lipgloss.Color("#8a9aaa")).Bold(true).Render(fmt.Sprintf("%d", d.stats.TotalNodes)),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#d4d4d4")).Render("Nodes"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#5a5a5a")).Render("executed"),
		))

	row.AddCells(totalCell, successCell, failCell, pendingCell, nodesCell)
	d.statsBox.SetRows([]*flexbox.Row{row})
	d.statsBox.ForceRecalculate()

	return d.statsBox.Render()
}

// renderPipelineDetail shows full pipeline details
func (d *DashboardView) renderPipelineDetail() string {
	run := d.selected
	if run == nil {
		return "Loading..."
	}

	var sections []string

	backBtn := d.theme.Base.Foreground(ColorMuted).Render("← ESC")
	statusIcon := GetStatusIcon(run.Status)
	statusStyle := d.theme.GetStatusStyle(run.Status)
	header := lipgloss.JoinHorizontal(lipgloss.Left,
		backBtn, "  ",
		statusStyle.Render(statusIcon), " ",
		d.theme.DetailHeader.Render(truncateString(run.RunID, 30)),
	)
	sections = append(sections, header)

	info := lipgloss.JoinVertical(lipgloss.Left,
		d.theme.DetailLabel.Render("Pipeline: ")+d.theme.DetailValue.Render(run.PipelineName),
		d.theme.DetailLabel.Render("Goal: ")+d.theme.DetailValue.Render(truncateString(run.Goal, d.width-15)),
		d.theme.DetailLabel.Render("Status: ")+statusStyle.Render(run.Status),
		d.theme.DetailLabel.Render("Nodes: ")+d.theme.DetailValue.Render(fmt.Sprintf("%d completed", run.NodeCount)),
	)
	sections = append(sections, "", info)

	timeline := d.renderTimeline()
	if timeline != "" {
		sections = append(sections, "", timeline)
	}

	nodeTable := d.renderNodeTable()
	sections = append(sections, "", d.theme.DetailSection.Render("Node Execution"), nodeTable)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderTimeline shows execution timeline
func (d *DashboardView) renderTimeline() string {
	if d.selected == nil || len(d.selected.Nodes) == 0 {
		return ""
	}

	nodes := d.selected.Nodes
	var parts []string

	for i, node := range nodes {
		icon := GetStatusIcon(node.Status)
		var style lipgloss.Style
		switch node.Status {
		case "success":
			style = d.theme.TimelineNodeDone
		case "fail":
			style = d.theme.TimelineNodeFailed
		default:
			style = d.theme.TimelineNodePending
		}

		parts = append(parts, style.Render(icon+" "+node.ID))
		if i < len(nodes)-1 {
			parts = append(parts, d.theme.TimelineEdge.Render("→"))
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		d.theme.DetailSection.Render("Timeline"),
		lipgloss.JoinHorizontal(lipgloss.Center, parts...),
	)
}

// renderNodeTable shows nodes in a simple format
func (d *DashboardView) renderNodeTable() string {
	if d.selected == nil || len(d.selected.Nodes) == 0 {
		return "No nodes"
	}

	var rows []string
	for _, node := range d.selected.Nodes {
		icon := GetStatusIcon(node.Status)
		style := d.theme.GetStatusStyle(node.Status)

		row := lipgloss.JoinHorizontal(lipgloss.Left,
			style.Render(icon), " ",
			d.theme.DetailValue.Render(fmt.Sprintf("%-15s", node.ID)),
			"  ",
			style.Render(node.Status),
		)
		rows = append(rows, row)
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// SetSize updates the view size
func (d *DashboardView) SetSize(width, height int) {
	d.width = width
	d.height = height

	headerHeight := 2
	cardsHeight := 5
	titleHeight := 2
	footerHeight := 2
	tableHeight := height - headerHeight - cardsHeight - titleHeight - footerHeight

	if tableHeight < 5 {
		tableHeight = 5
	}

	d.table.SetWidth(width)
	d.table.SetHeight(tableHeight)
}

// updateTableRows populates the table with run data
func (d *DashboardView) updateTableRows() {
	d.table.ClearRows()

	rows := make([][]any, len(d.runs))
	for i, run := range d.runs {
		runID := run.RunID
		if len(runID) > 20 {
			runID = runID[:17] + "..."
		}

		pipeline := run.PipelineName
		if len(pipeline) > 13 {
			pipeline = pipeline[:10] + "..."
		}

		status := GetStatusIcon(run.Status) + " " + run.Status

		rows[i] = []any{
			runID,
			pipeline,
			status,
			fmt.Sprintf("%d", run.NodeCount),
			FormatDurationHuman(time.Since(run.CompletedAt)),
		}
	}

	if len(rows) > 0 {
		d.table.MustAddRows(rows)
	}
}

// loadData loads runs from filesystem
func (d *DashboardView) loadData() tea.Cmd {
	return func() tea.Msg {
		runs, err := LoadRuns(d.logsRoot)
		if err != nil {
			return err
		}
		return dashboardRunsLoadedMsg{runs: runs}
	}
}

// loadRunDetail loads a single run
func (d *DashboardView) loadRunDetail(runID string) tea.Cmd {
	return func() tea.Msg {
		detail, err := LoadRun(d.logsRoot, runID)
		if err != nil {
			return err
		}
		return dashboardRunDetailLoadedMsg{detail: detail}
	}
}

// Message types
type dashboardRunsLoadedMsg struct {
	runs []RunSummary
}

type dashboardRunDetailLoadedMsg struct {
	detail *RunDetail
}
