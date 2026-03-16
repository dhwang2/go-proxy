package components

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SpinnerModel shows a spinner with a message during long operations.
type SpinnerModel struct {
	Message string
	spinner spinner.Model
	done    bool
	result  string
	err     error
}

// SpinnerDoneMsg signals that the operation completed.
type SpinnerDoneMsg struct {
	Result string
	Err    error
}

// NewSpinner creates a new spinner component.
func NewSpinner(message string) SpinnerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#00BCD4"))
	return SpinnerModel{
		Message: message,
		spinner: s,
	}
}

// Init implements tea.Model.
func (m SpinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update implements tea.Model.
func (m SpinnerModel) Update(msg tea.Msg) (SpinnerModel, tea.Cmd) {
	switch msg := msg.(type) {
	case SpinnerDoneMsg:
		m.done = true
		m.result = msg.Result
		m.err = msg.Err
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

// View implements tea.Model.
func (m SpinnerModel) View() string {
	if m.done {
		if m.err != nil {
			errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F44336"))
			return errStyle.Render("Error: " + m.err.Error())
		}
		okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#4CAF50"))
		return okStyle.Render(m.result)
	}
	return m.spinner.View() + " " + m.Message
}

// Done returns whether the operation has completed.
func (m SpinnerModel) Done() bool {
	return m.done
}
