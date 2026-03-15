package components

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Tab is an interface for tab components
type Tab interface {
	Init() tea.Cmd
	Update(tea.Msg) (Tab, tea.Cmd)
	View() string
	SetSize(width, height int)
	Title() string
}
