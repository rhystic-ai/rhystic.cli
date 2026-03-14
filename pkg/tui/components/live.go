package components

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// LiveTab shows live pipeline monitoring
type LiveTab struct {
	theme  Theme
	width  int
	height int
}

// NewLiveTab creates a new live monitoring tab
func NewLiveTab() *LiveTab {
	return &LiveTab{
		theme: NewTheme(),
	}
}

// Init initializes the tab
func (l *LiveTab) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (l *LiveTab) Update(msg tea.Msg) (Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		l.SetSize(msg.Width, msg.Height)
	}
	return l, nil
}

// View renders the tab
func (l *LiveTab) View() string {
	content := lipgloss.JoinVertical(
		lipgloss.Center,
		l.theme.PanelTitle.Render("Live Monitoring"),
		"",
		l.theme.Base.Foreground(colorMuted).Render("Live pipeline monitoring coming soon..."),
		"",
		l.theme.Base.Foreground(colorMuted).Render("This tab will show actively running pipelines"),
	)

	return l.theme.PanelBorder.
		Width(l.width).
		Height(l.height).
		Render(
			lipgloss.Place(
				l.width-4,
				l.height-2,
				lipgloss.Center,
				lipgloss.Center,
				content,
			),
		)
}

// SetSize updates the tab size
func (l *LiveTab) SetSize(width, height int) {
	l.width = width
	l.height = height
}

// Title returns the tab title
func (l *LiveTab) Title() string {
	return "Live"
}
