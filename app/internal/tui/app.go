package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/store"
)

// View is the interface each TUI view must implement.
type View interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (View, tea.Cmd)
	View() string
	Name() string
}

// Model is the root Bubble Tea model.
type Model struct {
	nav         NavState
	store       *store.Store
	version     string
	width       int
	height      int
	views       map[string]View
	overlay     OverlayModel
	current     string // name of the active view
	exitMessage string // printed to stdout after TUI exits
}

// NewModel creates the root model.
func NewModel(s *store.Store, version string) Model {
	return Model{
		store:   s,
		version: version,
		views:   make(map[string]View),
		width:   80,
		height:  24,
	}
}

// RegisterView adds a view to the model.
func (m *Model) RegisterView(v View) {
	m.views[v.Name()] = v
}

// Store returns the store for views to access.
func (m *Model) Store() *store.Store { return m.store }

// Version returns the version string.
func (m *Model) Version() string { return m.version }

// Width returns the terminal width.
func (m *Model) Width() int { return m.width }

// Height returns the terminal height.
func (m *Model) Height() int { return m.height }

// SetExitMessage sets a message to be printed to stdout after the TUI exits.
func (m *Model) SetExitMessage(msg string) { m.exitMessage = msg }

// ExitMessage returns the post-exit message (empty if none).
func (m Model) ExitMessage() string { return m.exitMessage }

// Init satisfies tea.Model.
func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		return NavigateMsg{Name: "main-menu"}
	}
}

// Update handles messages at the root level.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case NavigateMsg:
		m.nav.Push(msg.Name)
		m.current = msg.Name
		m.overlay = nil
		if v, ok := m.views[msg.Name]; ok {
			cmd := v.Init()
			return m, cmd
		}
		return m, nil

	case BackMsg:
		if m.overlay != nil {
			m.overlay = nil
			return m, nil
		}
		name := m.nav.Pop()
		if name != m.current {
			m.current = name
			if v, ok := m.views[name]; ok {
				cmd := v.Init()
				return m, cmd
			}
		}
		return m, nil

	case ShowOverlayMsg:
		m.overlay = msg.Overlay
		if m.overlay != nil {
			cmd := m.overlay.Init()
			return m, cmd
		}
		return m, nil

	case DismissOverlayMsg:
		m.overlay = nil
		return m, nil

	case InputResultMsg, ConfirmResultMsg, ResultDismissedMsg, OverlaySelectMsg:
		// Overlay result messages: dismiss overlay and route to current view.
		m.overlay = nil
		if v, ok := m.views[m.current]; ok {
			newView, cmd := v.Update(msg)
			m.views[m.current] = newView
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		// Ctrl+C always quits, even with an overlay active.
		if key.Matches(msg, Keys.Quit) {
			return m, tea.Quit
		}

		// If overlay is active, delegate to overlay.
		if m.overlay != nil {
			var cmd tea.Cmd
			m.overlay, cmd = m.overlay.Update(msg)
			return m, cmd
		}

		// Global keys (only when no overlay).
		switch {
		case key.Matches(msg, Keys.QuitQ):
			if m.current == "main-menu" {
				return m, tea.Quit
			}
		case key.Matches(msg, Keys.Back):
			if m.current == "main-menu" {
				return m, tea.Quit
			}
			if m.nav.Depth() > 1 {
				return m, BackCmd
			}
			return m, nil
		}
	}

	// When overlay is active, route messages to both the view and the overlay.
	// The view needs to receive async completion messages (e.g., coreVersionsDoneMsg)
	// even while a spinner overlay is showing. The overlay needs tick messages.
	if m.overlay != nil {
		var cmds []tea.Cmd

		// Route to the current view first so it can handle completion messages.
		if v, ok := m.views[m.current]; ok {
			newView, cmd := v.Update(msg)
			m.views[m.current] = newView
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

		// Also route to the overlay for tick/animation messages.
		var overlayCmd tea.Cmd
		m.overlay, overlayCmd = m.overlay.Update(msg)
		if overlayCmd != nil {
			cmds = append(cmds, overlayCmd)
		}

		return m, tea.Batch(cmds...)
	}

	// Delegate to current view.
	if v, ok := m.views[m.current]; ok {
		newView, cmd := v.Update(msg)
		m.views[m.current] = newView
		return m, cmd
	}

	return m, nil
}

// View renders the current state.
func (m Model) View() string {
	var content string
	if v, ok := m.views[m.current]; ok {
		content = v.View()
	}

	if m.overlay != nil {
		overlayContent := m.overlay.View()
		// Center the overlay on the screen.
		content = lipgloss.Place(
			m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			overlayContent,
			lipgloss.WithWhitespaceChars(" "),
		)
	}

	return content
}
