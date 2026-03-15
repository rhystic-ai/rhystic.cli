package tui

import (
	"github.com/76creates/stickers/table"
	"github.com/charmbracelet/lipgloss"
)

// Color palette - Professional Dark Theme
// Muted, business-like colors with good readability
var (
	ColorBackground = lipgloss.Color("#1a1a1a") // Near black background
	ColorSurface    = lipgloss.Color("#252525") // Dark gray surface
	ColorText       = lipgloss.Color("#d4d4d4") // Soft white text
	ColorPrimary    = lipgloss.Color("#5a7a8c") // Muted blue-gray
	ColorSecondary  = lipgloss.Color("#6b8a9e") // Lighter blue-gray
	ColorAccent     = lipgloss.Color("#7d8b96") // Slate accent
	ColorSuccess    = lipgloss.Color("#5a8a6a") // Muted green
	ColorWarning    = lipgloss.Color("#8a7a5a") // Muted yellow/olive
	ColorError      = lipgloss.Color("#8a5a5a") // Muted red
	ColorMuted      = lipgloss.Color("#5a5a5a") // Gray for muted text
)

// Theme holds all styles for the TUI
type Theme struct {
	Base        lipgloss.Style
	Header      lipgloss.Style
	HeaderTitle lipgloss.Style
	TabActive   lipgloss.Style
	TabInactive lipgloss.Style
	Help        lipgloss.Style

	StatusSuccess lipgloss.Style
	StatusError   lipgloss.Style
	StatusWarning lipgloss.Style
	StatusPending lipgloss.Style

	PanelBorder   lipgloss.Style
	PanelTitle    lipgloss.Style
	DetailHeader  lipgloss.Style
	DetailSection lipgloss.Style
	DetailLabel   lipgloss.Style
	DetailValue   lipgloss.Style

	TimelineNodeDone    lipgloss.Style
	TimelineNodeFailed  lipgloss.Style
	TimelineNodePending lipgloss.Style
	TimelineEdge        lipgloss.Style

	CardStyle lipgloss.Style
}

// NewTheme creates a new theme
func NewTheme() Theme {
	return Theme{
		Base: lipgloss.NewStyle().
			Foreground(ColorText).
			Background(ColorBackground),

		Header: lipgloss.NewStyle().
			Background(ColorSurface).
			Foreground(lipgloss.Color("#7d8b96")).
			Padding(0, 1).
			Bold(true),

		HeaderTitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8a9aa6")).
			Bold(true),

		TabActive: lipgloss.NewStyle().
			Foreground(ColorBackground).
			Background(lipgloss.Color("#5a7a8c")).
			Padding(0, 2).
			Bold(true),

		TabInactive: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#5a5a5a")).
			Background(ColorSurface).
			Padding(0, 2),

		Help: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6a6a6a")).
			Background(ColorSurface).
			Padding(0, 1),

		StatusSuccess: lipgloss.NewStyle().Foreground(lipgloss.Color("#6a9a7a")),
		StatusError:   lipgloss.NewStyle().Foreground(lipgloss.Color("#9a6a6a")),
		StatusWarning: lipgloss.NewStyle().Foreground(lipgloss.Color("#9a8a6a")),
		StatusPending: lipgloss.NewStyle().Foreground(lipgloss.Color("#5a5a5a")),

		PanelBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#4a4a4a")).
			Padding(0, 1),

		PanelTitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8a9aa6")).
			Bold(true),

		DetailHeader: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8a9aa6")).
			Bold(true).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(lipgloss.Color("#4a4a4a")),

		DetailSection: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7a8a96")).
			Bold(true),

		DetailLabel: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#5a7a8c")),

		DetailValue: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#c4c4c4")),

		TimelineNodeDone:    lipgloss.NewStyle().Foreground(lipgloss.Color("#6a9a7a")),
		TimelineNodeFailed:  lipgloss.NewStyle().Foreground(lipgloss.Color("#9a6a6a")).Bold(true),
		TimelineNodePending: lipgloss.NewStyle().Foreground(lipgloss.Color("#5a5a5a")),
		TimelineEdge:        lipgloss.NewStyle().Foreground(lipgloss.Color("#5a5a5a")),

		CardStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#4a4a4a")).
			Padding(1, 2).
			Align(lipgloss.Center),
	}
}

// GetStatusStyle returns the appropriate style for a status
func (t Theme) GetStatusStyle(status string) lipgloss.Style {
	switch status {
	case "success":
		return t.StatusSuccess
	case "partial_success":
		return t.StatusWarning
	case "fail":
		return t.StatusError
	default:
		return t.StatusPending
	}
}

// GetStatusIcon returns the icon for a status
func GetStatusIcon(status string) string {
	switch status {
	case "success":
		return "✓"
	case "partial_success":
		return "~"
	case "fail":
		return "✗"
	case "running":
		return "►"
	default:
		return "○"
	}
}

// GetTableStyles returns stickers table styles with professional dark theme
func GetTableStyles() map[table.StyleKey]lipgloss.Style {
	return map[table.StyleKey]lipgloss.Style{
		table.StyleKeyHeader: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#1a1a1a")).
			Background(lipgloss.Color("#6a7a8a")).
			Bold(true).
			Padding(0, 1),
		table.StyleKeyFooter: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7a8a96")).
			Background(lipgloss.Color("#252525")).
			Align(lipgloss.Right).
			Padding(0, 1),
		table.StyleKeyRows: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#c4c4c4")).
			Background(lipgloss.Color("#1a1a1a")).
			Padding(0, 1),
		table.StyleKeyRowsSubsequent: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#c4c4c4")).
			Background(lipgloss.Color("#252525")).
			Padding(0, 1),
		table.StyleKeyRowsCursor: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#1a1a1a")).
			Background(lipgloss.Color("#7a8a96")).
			Bold(true).
			Padding(0, 1),
		table.StyleKeyCellCursor: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#1a1a1a")).
			Background(lipgloss.Color("#8a9aaa")).
			Bold(true).
			Padding(0, 1),
	}
}
