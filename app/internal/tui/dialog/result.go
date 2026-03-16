package dialog

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// NewResult creates a modal result display with an OK button.
func NewResult(message string, onDone func()) tview.Primitive {
	modal := tview.NewModal().
		SetText(message).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(_ int, _ string) {
			onDone()
		}).
		SetBackgroundColor(tcell.ColorDarkSlateGray)
	modal.SetBorder(true).
		SetBorderColor(tcell.ColorTeal)
	return modal
}
