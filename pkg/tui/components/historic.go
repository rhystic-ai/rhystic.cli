package components

import (
	"fmt"
	"io"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Color definitions matching professional theme
var (
	colorPrimary    = lipgloss.Color("#5a7a8c")
	colorSuccess    = lipgloss.Color("#5a8a6a")
	colorWarning    = lipgloss.Color("#8a7a5a")
	colorError      = lipgloss.Color("#8a5a5a")
	colorBackground = lipgloss.Color("#1a1a1a")
	colorText       = lipgloss.Color("#d4d4d4")
	colorMuted      = lipgloss.Color("#5a5a5a")
)

// Theme holds all styles
type Theme struct {
	Base                lipgloss.Style
	RunListItem         lipgloss.Style
	RunListTitle        lipgloss.Style
	RunListMeta         lipgloss.Style
	RunListSelected     lipgloss.Style
	PanelBorder         lipgloss.Style
	PanelTitle          lipgloss.Style
	DetailHeader        lipgloss.Style
	DetailSection       lipgloss.Style
	DetailLabel         lipgloss.Style
	DetailValue         lipgloss.Style
	TimelineNodeDone    lipgloss.Style
	TimelineNodeFailed  lipgloss.Style
	TimelineNodePending lipgloss.Style
	TimelineEdge        lipgloss.Style
}

// NewTheme creates a theme matching the main tui theme
func NewTheme() Theme {
	return Theme{
		Base: lipgloss.NewStyle().
			Foreground(colorText).
			Background(colorBackground),
		RunListItem: lipgloss.NewStyle().
			Padding(0, 1),
		RunListTitle: lipgloss.NewStyle().
			Foreground(colorText).
			Bold(true),
		RunListMeta: lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true),
		RunListSelected: lipgloss.NewStyle().
			Foreground(colorBackground).
			Background(colorPrimary).
			Padding(0, 1),
		PanelBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorMuted).
			Padding(1),
		PanelTitle: lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true).
			MarginBottom(1),
		DetailHeader: lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true).
			MarginBottom(1).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(colorMuted),
		DetailSection: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7a8a96")).
			Bold(true).
			MarginTop(1).
			MarginBottom(1),
		DetailLabel: lipgloss.NewStyle().
			Foreground(colorMuted),
		DetailValue: lipgloss.NewStyle().
			Foreground(colorText),
		TimelineNodeDone: lipgloss.NewStyle().
			Foreground(colorSuccess),
		TimelineNodeFailed: lipgloss.NewStyle().
			Foreground(colorError).
			Bold(true),
		TimelineNodePending: lipgloss.NewStyle().
			Foreground(colorMuted),
		TimelineEdge: lipgloss.NewStyle().
			Foreground(colorMuted),
	}
}

// GetStatusStyle returns style for status
func (t Theme) GetStatusStyle(status string) lipgloss.Style {
	switch status {
	case "success":
		return lipgloss.NewStyle().Foreground(colorSuccess)
	case "partial_success":
		return lipgloss.NewStyle().Foreground(colorWarning)
	case "fail":
		return lipgloss.NewStyle().Foreground(colorError)
	default:
		return lipgloss.NewStyle().Foreground(colorMuted)
	}
}

// GetStatusIcon returns icon for status
func GetStatusIcon(status string) string {
	switch status {
	case "success":
		return "●"
	case "partial_success":
		return "◐"
	case "fail":
		return "◍"
	default:
		return "○"
	}
}

// RunItem wraps RunSummary for the list component
type RunItem struct {
	Run RunSummary
}

func (i RunItem) FilterValue() string {
	return fmt.Sprintf("%s %s %s", i.Run.RunID, i.Run.PipelineName, i.Run.Goal)
}

// RunDelegate renders run items in the list
type RunDelegate struct {
	theme Theme
}

func (d RunDelegate) Height() int  { return 3 }
func (d RunDelegate) Spacing() int { return 1 }

func (d RunDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

func (d RunDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	run, ok := item.(RunItem)
	if !ok {
		return
	}

	var style lipgloss.Style
	if index == m.Index() {
		style = d.theme.RunListSelected
	} else {
		style = d.theme.RunListItem
	}

	statusIcon := GetStatusIcon(run.Run.Status)
	statusStyle := d.theme.GetStatusStyle(run.Run.Status)

	line1 := lipgloss.JoinHorizontal(
		lipgloss.Left,
		statusStyle.Render(statusIcon),
		" ",
		d.theme.RunListTitle.Render(truncateString(run.Run.RunID, 25)),
	)

	line2 := d.theme.RunListMeta.Render(
		fmt.Sprintf("%s • %s • %d nodes",
			truncateString(run.Run.PipelineName, 20),
			formatDuration(time.Since(run.Run.CompletedAt)),
			run.Run.NodeCount,
		),
	)

	if run.Run.Goal != "" {
		line3 := d.theme.Base.Foreground(colorMuted).Render(
			truncateString(run.Run.Goal, m.Width()-4),
		)
		w.Write([]byte(style.Render(lipgloss.JoinVertical(lipgloss.Left, line1, line2, line3))))
	} else {
		w.Write([]byte(style.Render(lipgloss.JoinVertical(lipgloss.Left, line1, line2))))
	}
}

// HistoricTab shows completed pipeline runs
type HistoricTab struct {
	logsRoot string
	theme    Theme
	width    int
	height   int

	list        list.Model
	runs        []RunSummary
	selectedRun *RunDetail
	showDetail  bool
}

// NewHistoricTab creates a new historic runs tab
func NewHistoricTab(logsRoot string) *HistoricTab {
	delegate := RunDelegate{theme: NewTheme()}
	items := []list.Item{}

	l := list.New(items, delegate, 50, 20)
	l.Title = "Pipeline Runs"
	l.SetShowTitle(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(true)
	l.Styles.StatusBar = lipgloss.NewStyle().
		Foreground(colorMuted).
		Background(colorBackground)

	return &HistoricTab{
		logsRoot: logsRoot,
		theme:    NewTheme(),
		list:     l,
	}
}

// Init initializes the tab
func (h *HistoricTab) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (h *HistoricTab) Update(msg tea.Msg) (Tab, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.SetSize(msg.Width, msg.Height)

	case tea.KeyMsg:
		if !h.showDetail {
			switch msg.String() {
			case "enter":
				if item, ok := h.list.SelectedItem().(RunItem); ok {
					cmds = append(cmds, h.loadRunDetail(item.Run.RunID))
				}
			}
		} else {
			switch msg.String() {
			case "esc", "backspace":
				h.showDetail = false
				h.selectedRun = nil
			}
		}
	}

	if !h.showDetail {
		newList, cmd := h.list.Update(msg)
		h.list = newList
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return h, tea.Batch(cmds...)
}

// View renders the tab
func (h *HistoricTab) View() string {
	if h.showDetail && h.selectedRun != nil {
		return h.renderDetailView()
	}
	return h.renderListView()
}

// renderListView shows the run list
func (h *HistoricTab) renderListView() string {
	if len(h.runs) == 0 {
		return h.theme.PanelBorder.
			Width(h.width).
			Height(h.height).
			Render(
				lipgloss.Place(
					h.width-4,
					h.height-2,
					lipgloss.Center,
					lipgloss.Center,
					h.theme.Base.Foreground(colorMuted).Render("No pipeline runs found"),
				),
			)
	}

	// Split view: list on left, preview on right (if wide enough)
	if h.width >= 100 {
		listWidth := h.width / 3
		previewWidth := h.width - listWidth - 2

		h.list.SetSize(listWidth, h.height)
		listView := h.theme.PanelBorder.
			Width(listWidth).
			Height(h.height).
			Render(h.list.View())

		preview := h.renderPreviewPanel(previewWidth, h.height)

		return lipgloss.JoinHorizontal(lipgloss.Top, listView, preview)
	}

	// Single column layout
	h.list.SetSize(h.width, h.height)
	return h.theme.PanelBorder.
		Width(h.width).
		Height(h.height).
		Render(h.list.View())
}

// renderPreviewPanel shows a preview of the selected run
func (h *HistoricTab) renderPreviewPanel(width, height int) string {
	item, ok := h.list.SelectedItem().(RunItem)
	if !ok {
		return h.theme.PanelBorder.
			Width(width).
			Height(height).
			Render(
				lipgloss.Place(
					width-4,
					height-2,
					lipgloss.Center,
					lipgloss.Center,
					h.theme.Base.Foreground(colorMuted).Render("Select a run to preview"),
				),
			)
	}

	run := item.Run
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		h.theme.PanelTitle.Render("Run Preview"),
		"",
		h.theme.DetailLabel.Render("Run ID: ")+h.theme.DetailValue.Render(run.RunID),
		h.theme.DetailLabel.Render("Pipeline: ")+h.theme.DetailValue.Render(run.PipelineName),
		h.theme.DetailLabel.Render("Goal: ")+h.theme.DetailValue.Render(truncateString(run.Goal, width-10)),
		"",
		h.theme.DetailLabel.Render("Status: ")+h.renderStatus(run.Status),
		h.theme.DetailLabel.Render("Nodes: ")+h.theme.DetailValue.Render(fmt.Sprintf("%d completed", run.NodeCount)),
		h.theme.DetailLabel.Render("Time: ")+h.theme.DetailValue.Render(formatDuration(time.Since(run.CompletedAt))+" ago"),
		"",
		h.theme.Base.Foreground(colorMuted).Render("Press Enter to view details"),
	)

	return h.theme.PanelBorder.
		Width(width).
		Height(height).
		Render(content)
}

// renderDetailView shows full run details
func (h *HistoricTab) renderDetailView() string {
	run := h.selectedRun
	if run == nil {
		return "Loading..."
	}

	// Header
	header := h.theme.DetailHeader.Render(
		lipgloss.JoinHorizontal(
			lipgloss.Left,
			h.theme.GetStatusStyle(run.Status).Render(GetStatusIcon(run.Status)),
			" ",
			run.RunID,
		),
	)

	// Info section
	info := lipgloss.JoinVertical(
		lipgloss.Left,
		h.theme.DetailLabel.Render("Pipeline: ")+h.theme.DetailValue.Render(run.PipelineName),
		h.theme.DetailLabel.Render("Goal: ")+h.theme.DetailValue.Render(run.Goal),
		h.theme.DetailLabel.Render("Current Node: ")+h.theme.DetailValue.Render(run.CurrentNode),
	)

	// Timeline section
	timeline := h.renderTimeline()

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		"",
		info,
		"",
		timeline,
		"",
		h.theme.Base.Foreground(colorMuted).Render("ESC to go back"),
	)

	return h.theme.PanelBorder.
		Width(h.width).
		Height(h.height).
		Render(content)
}

// renderTimeline shows the execution timeline
func (h *HistoricTab) renderTimeline() string {
	if h.selectedRun == nil || len(h.selectedRun.Nodes) == 0 {
		return ""
	}

	var nodes []string
	for _, node := range h.selectedRun.Nodes {
		icon := GetStatusIcon(node.Status)
		var style lipgloss.Style
		switch node.Status {
		case "success":
			style = h.theme.TimelineNodeDone
		case "fail":
			style = h.theme.TimelineNodeFailed
		default:
			style = h.theme.TimelineNodePending
		}

		nodeStr := style.Render(icon + " " + node.ID)
		nodes = append(nodes, nodeStr)
	}

	timeline := lipgloss.JoinHorizontal(
		lipgloss.Center,
		nodes[0],
	)
	for i := 1; i < len(nodes); i++ {
		timeline = lipgloss.JoinHorizontal(
			lipgloss.Center,
			timeline,
			h.theme.TimelineEdge.Render(" → "),
			nodes[i],
		)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		h.theme.DetailSection.Render("Execution Timeline"),
		timeline,
	)
}

// renderStatus renders the status with color
func (h *HistoricTab) renderStatus(status string) string {
	style := h.theme.GetStatusStyle(status)
	return style.Render(status)
}

// SetSize updates the tab size
func (h *HistoricTab) SetSize(width, height int) {
	h.width = width
	h.height = height
	if !h.showDetail {
		h.list.SetSize(width, height)
	}
}

// SetRuns updates the runs in the list
func (h *HistoricTab) SetRuns(runs []RunSummary) {
	h.runs = runs
	items := make([]list.Item, len(runs))
	for i, run := range runs {
		items[i] = RunItem{Run: run}
	}
	h.list.SetItems(items)
	h.list.SetSize(h.width, h.height)
}

// Title returns the tab title
func (h *HistoricTab) Title() string {
	return "Historic"
}

// loadRunDetail loads detailed run data
func (h *HistoricTab) loadRunDetail(runID string) tea.Cmd {
	return func() tea.Msg {
		detail, err := h.loadRun(runID)
		if err != nil {
			return err
		}
		h.selectedRun = detail
		h.showDetail = true
		return runDetailLoadedMsg{}
	}
}

type runDetailLoadedMsg struct{}

// loadRun loads run details from filesystem
func (h *HistoricTab) loadRun(runID string) (*RunDetail, error) {
	// For now, return a placeholder
	// In full implementation, this would read from filesystem
	return &RunDetail{
		RunSummary: RunSummary{
			RunID: runID,
		},
	}, nil
}

// Helper functions
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}
