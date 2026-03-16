package tui

import "github.com/rivo/tview"

// Page is the interface each TUI page must implement.
type Page interface {
	Name() string
	Root() tview.Primitive
	OnEnter()
}
