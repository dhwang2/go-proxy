package dialog

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// NewConfirm creates a modal yes/no dialog.
// onDone is called with true (yes) or false (no/cancel).
func NewConfirm(prompt string, onDone func(bool)) tview.Primitive {
	modal := tview.NewModal().
		SetText(prompt).
		AddButtons([]string{"Yes", "No"}).
		SetDoneFunc(func(_ int, label string) {
			onDone(label == "Yes")
		}).
		SetBackgroundColor(tcell.ColorDarkSlateGray)
	modal.SetBorder(true).
		SetBorderColor(tcell.ColorTeal)
	return modal
}
