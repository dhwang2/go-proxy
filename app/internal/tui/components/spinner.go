package components

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Spinner struct {
	model   spinner.Model
	message string
}

func NewSpinner(message string) Spinner {
	m := spinner.New()
	m.Spinner = spinner.Spinner{
		Frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		FPS:    time.Second / 12,
	}
	m.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffd75f")).Bold(true)
	return Spinner{model: m, message: message}
}

func (s Spinner) Init() tea.Cmd {
	return s.model.Tick
}

func (s Spinner) Update(msg tea.Msg) (Spinner, tea.Cmd) {
	model, cmd := s.model.Update(msg)
	s.model = model
	return s, cmd
}

func (s Spinner) View() string {
	if s.message == "" {
		return s.model.View()
	}
	return fmt.Sprintf("%s %s", s.model.View(), s.message)
}

func (s *Spinner) SetMessage(message string) {
	s.message = message
}
