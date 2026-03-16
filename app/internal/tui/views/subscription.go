package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"go-proxy/internal/derived"
	"go-proxy/internal/subscription"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type SubscriptionView struct {
	model       *tui.Model
	menu        components.MenuModel
	step        subStep
	pendingUser string
}

type subStep int

const (
	subMenu subStep = iota
	subFormat
	subResult
)

func NewSubscriptionView(model *tui.Model) *SubscriptionView {
	return &SubscriptionView{model: model}
}

func (v *SubscriptionView) Name() string { return "subscription" }

func (v *SubscriptionView) Init() tea.Cmd {
	v.step = subMenu
	names := derived.UserNames(v.model.Store())
	if len(names) == 0 {
		return func() tea.Msg {
			return tui.ShowOverlayMsg{
				Overlay: components.NewResult("No users found"),
			}
		}
	}
	items := make([]components.MenuItem, 0, len(names)+1)
	for i, name := range names {
		k := rune('1' + i)
		if i >= 9 {
			k = rune('a' + i - 9)
		}
		items = append(items, components.MenuItem{Key: k, Label: name, ID: name})
	}
	items = append(items, components.MenuItem{Key: '0', Label: "返回  Back", ID: "back"})
	v.menu = v.menu.SetItems(items)
	return nil
}

func (v *SubscriptionView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case components.MenuSelectMsg:
		if msg.ID == "back" {
			return v, func() tea.Msg { return tui.BackMsg{} }
		}
		if v.step == subMenu {
			v.pendingUser = msg.ID
			v.step = subFormat
			formatMenu := components.NewMenu("Select Format", []components.MenuItem{
				{Key: '1', Label: "Surge", ID: "surge"},
				{Key: '2', Label: "sing-box", ID: "singbox"},
				{Key: '0', Label: "返回  Back", ID: "back"},
			})
			return v, func() tea.Msg {
				return tui.ShowOverlayMsg{Overlay: subFormatOverlay{menu: formatMenu}}
			}
		}
		return v, nil

	case subFormatSelectMsg:
		if msg.id == "back" {
			v.step = subMenu
			return v, func() tea.Msg { return tui.DismissOverlayMsg{} }
		}
		var format subscription.Format
		if msg.id == "singbox" {
			format = subscription.FormatSingBox
		} else {
			format = subscription.FormatSurge
		}
		user := v.pendingUser
		links := subscription.Render(v.model.Store(), user, format, "")
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Subscription: %s (%s)\n\n", user, format))
		for _, l := range links {
			sb.WriteString(l.Tag + "\n")
			sb.WriteString(l.Content + "\n\n")
		}
		if len(links) == 0 {
			sb.WriteString("No subscriptions available")
		}
		v.step = subResult
		result := sb.String()
		return v, func() tea.Msg {
			return tui.ShowOverlayMsg{
				Overlay: components.NewResult(result),
			}
		}

	case tui.ResultDismissedMsg:
		v.step = subMenu
		return v, func() tea.Msg { return tui.DismissOverlayMsg{} }

	default:
		if v.step == subMenu {
			var cmd tea.Cmd
			v.menu, cmd = v.menu.Update(msg)
			return v, cmd
		}
	}
	return v, nil
}

func (v *SubscriptionView) View() string { return v.menu.View() }

type subFormatSelectMsg struct{ id string }

type subFormatOverlay struct {
	menu components.MenuModel
}

func (o subFormatOverlay) Init() tea.Cmd { return nil }

func (o subFormatOverlay) Update(msg tea.Msg) (tui.OverlayModel, tea.Cmd) {
	switch msg := msg.(type) {
	case components.MenuSelectMsg:
		id := msg.ID
		return o, func() tea.Msg { return subFormatSelectMsg{id: id} }
	default:
		var cmd tea.Cmd
		o.menu, cmd = o.menu.Update(msg)
		return o, cmd
	}
}

func (o subFormatOverlay) View() string {
	return tui.DialogStyle.Render(o.menu.View())
}
