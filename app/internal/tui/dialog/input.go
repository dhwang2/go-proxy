package dialog

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// NewInput creates a modal text input dialog.
// onDone is called with the entered value; empty string means cancelled.
func NewInput(prompt, placeholder string, onDone func(string)) tview.Primitive {
	form := tview.NewForm().
		AddInputField(prompt, placeholder, 30, nil, nil).
		AddButton("OK", nil).
		AddButton("Cancel", nil)

	form.SetBorder(true).
		SetTitle(" " + prompt + " ").
		SetTitleColor(tcell.ColorTeal).
		SetBorderColor(tcell.ColorTeal).
		SetBackgroundColor(tcell.ColorDarkSlateGray)

	form.SetButtonsAlign(tview.AlignCenter)

	form.GetButton(0).SetSelectedFunc(func() {
		val := form.GetFormItem(0).(*tview.InputField).GetText()
		onDone(val)
	})
	form.GetButton(1).SetSelectedFunc(func() {
		onDone("")
	})

	form.SetCancelFunc(func() {
		onDone("")
	})

	// Center the form in a flex layout.
	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(form, 9, 0, true).
			AddItem(nil, 0, 1, false), 50, 0, true).
		AddItem(nil, 0, 1, false)

	return flex
}
