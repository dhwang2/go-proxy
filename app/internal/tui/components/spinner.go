package components

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/tui"
)

// SpinnerModel wraps bubbles/spinner for async operations.
type SpinnerModel struct {
	spinner      spinner.Model
	message      string
	elapsed      int
	totalSeconds int
}

// NewSpinner creates a new spinner overlay.
func NewSpinner(message string) SpinnerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(tui.ColorPrimary)
	return SpinnerModel{spinner: s, message: message}
}

// NewTimedSpinner creates a spinner that also shows elapsed seconds.
func NewTimedSpinner(message string, totalSeconds int) SpinnerModel {
	m := NewSpinner(message)
	m.totalSeconds = totalSeconds
	return m
}

type spinnerElapsedMsg struct{}

func (m SpinnerModel) Init() tea.Cmd {
	if m.totalSeconds > 0 {
		return tea.Batch(m.spinner.Tick, nextSpinnerElapsed())
	}
	return m.spinner.Tick
}

func (m SpinnerModel) Update(msg tea.Msg) (tui.OverlayModel, tea.Cmd) {
	switch msg.(type) {
	case spinnerElapsedMsg:
		m.elapsed++
		return m, nextSpinnerElapsed()
	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
}

func (m SpinnerModel) View() string {
	label := m.message
	if m.totalSeconds > 0 {
		label = fmt.Sprintf("%s %ds/%ds", m.message, m.elapsed, m.totalSeconds)
	}
	content := lipgloss.JoinVertical(lipgloss.Center,
		m.spinner.View()+" "+label,
	)
	return tui.DialogStyle.Render(content)
}

func nextSpinnerElapsed() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return spinnerElapsedMsg{}
	})
}
