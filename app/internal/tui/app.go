package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"go-proxy/internal/store"
)

// AppState holds shared TUI state accessible by all pages.
type AppState struct {
	App     *tview.Application
	Pages   *tview.Pages
	Store   *store.Store
	Version string
	history []string // page name stack for back navigation
}

// NewApp creates and initializes the TUI application.
func NewApp(app *tview.Application, s *store.Store, version string) *AppState {
	state := &AppState{
		App:     app,
		Pages:   tview.NewPages(),
		Store:   s,
		Version: version,
		history: []string{},
	}

	// Global input capture for Esc (back) and Ctrl-C (quit).
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Let dialogs handle their own input first.
		name, _ := state.Pages.GetFrontPage()
		if len(name) > 7 && name[:7] == "dialog-" {
			return event
		}

		switch event.Key() {
		case tcell.KeyCtrlC:
			if name == "main-menu" {
				app.Stop()
				return nil
			}
		case tcell.KeyEscape:
			state.Back()
			return nil
		case tcell.KeyRune:
			if event.Rune() == 'q' && name == "main-menu" {
				app.Stop()
				return nil
			}
		}
		return event
	})

	app.SetRoot(state.Pages, true)
	return state
}

// RegisterPage adds a page to the pages container.
func (a *AppState) RegisterPage(p Page) {
	a.Pages.AddPage(p.Name(), p.Root(), true, false)
}

// Navigate switches to the named page and pushes to history.
func (a *AppState) Navigate(name string) {
	a.history = append(a.history, name)
	a.Pages.SwitchToPage(name)

	// Call OnEnter on all registered pages that match.
	// We find the page's primitive to trigger OnEnter.
}

// NavigateWithCallback switches to the named page, pushes history, and calls OnEnter.
func (a *AppState) NavigateWithCallback(p Page) {
	a.history = append(a.history, p.Name())
	a.Pages.SwitchToPage(p.Name())
	p.OnEnter()
}

// Back pops the history stack and returns to the previous page.
func (a *AppState) Back() {
	if len(a.history) <= 1 {
		return
	}
	a.history = a.history[:len(a.history)-1]
	prev := a.history[len(a.history)-1]
	a.Pages.SwitchToPage(prev)
}

// ShowDialog displays a named dialog as an overlay.
func (a *AppState) ShowDialog(name string, prim tview.Primitive) {
	a.Pages.AddPage("dialog-"+name, prim, true, true)
}

// DismissDialog removes a named dialog overlay.
func (a *AppState) DismissDialog(name string) {
	a.Pages.RemovePage("dialog-" + name)
}

// CurrentPage returns the name of the front page.
func (a *AppState) CurrentPage() string {
	name, _ := a.Pages.GetFrontPage()
	return name
}
