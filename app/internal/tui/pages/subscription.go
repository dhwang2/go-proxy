package pages

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"go-proxy/internal/derived"
	"go-proxy/internal/subscription"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/dialog"
)

// SubscriptionPage handles subscription link display.
type SubscriptionPage struct {
	state *tui.AppState
	list  *tview.List
}

// NewSubscriptionPage creates the subscription page.
func NewSubscriptionPage(state *tui.AppState) *SubscriptionPage {
	p := &SubscriptionPage{state: state}

	p.list = tview.NewList().
		ShowSecondaryText(false).
		SetHighlightFullLine(true).
		SetMainTextColor(tcell.ColorWhite).
		SetSelectedTextColor(tcell.ColorTeal).
		SetSelectedBackgroundColor(tcell.ColorDarkSlateGray)
	p.list.SetTitle(" Subscription ").
		SetTitleColor(tcell.ColorTeal).
		SetBorder(true).
		SetBorderColor(tcell.ColorTeal)

	return p
}

func (p *SubscriptionPage) Name() string          { return "subscription" }
func (p *SubscriptionPage) Root() tview.Primitive { return p.list }

func (p *SubscriptionPage) OnEnter() {
	p.list.Clear()
	names := derived.UserNames(p.state.Store)
	if len(names) == 0 {
		p.showResult("No users found")
		return
	}
	for i, name := range names {
		userName := name
		key := rune('1' + i)
		if i >= 9 {
			key = rune('a' + i - 9)
		}
		p.list.AddItem(userName, "", key, func() {
			p.selectFormat(userName)
		})
	}
	p.list.AddItem("返回  Back", "", '0', func() {
		p.state.Back()
	})
	p.state.App.SetFocus(p.list)
}

func (p *SubscriptionPage) selectFormat(userName string) {
	formatList := tview.NewList().
		ShowSecondaryText(false).
		SetHighlightFullLine(true).
		SetMainTextColor(tcell.ColorWhite).
		SetSelectedTextColor(tcell.ColorTeal).
		SetSelectedBackgroundColor(tcell.ColorDarkSlateGray)
	formatList.SetTitle(" Select Format ").
		SetTitleColor(tcell.ColorTeal).
		SetBorder(true).
		SetBorderColor(tcell.ColorTeal)

	formatList.AddItem("Surge", "", '1', func() {
		p.state.DismissDialog("sub-format")
		p.renderLinks(userName, subscription.FormatSurge)
	})
	formatList.AddItem("sing-box", "", '2', func() {
		p.state.DismissDialog("sub-format")
		p.renderLinks(userName, subscription.FormatSingBox)
	})
	formatList.AddItem("返回  Back", "", '0', func() {
		p.state.DismissDialog("sub-format")
	})

	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(formatList, 7, 0, true).
			AddItem(nil, 0, 1, false), 35, 0, true).
		AddItem(nil, 0, 1, false)

	p.state.ShowDialog("sub-format", flex)
}

func (p *SubscriptionPage) renderLinks(userName string, format subscription.Format) {
	links := subscription.Render(p.state.Store, userName, format, "")
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Subscription: %s (%s)\n\n", userName, format))
	for _, l := range links {
		sb.WriteString(l.Tag + "\n")
		sb.WriteString(l.Content + "\n\n")
	}
	if len(links) == 0 {
		sb.WriteString("No subscriptions available")
	}
	p.showResult(sb.String())
}

func (p *SubscriptionPage) showResult(msg string) {
	d := dialog.NewResult(msg, func() {
		p.state.DismissDialog("sub-result")
	})
	p.state.ShowDialog("sub-result", d)
}
