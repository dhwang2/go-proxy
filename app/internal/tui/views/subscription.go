package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/derived"
	"go-proxy/internal/subscription"
	"go-proxy/internal/tui"
)

type SubscriptionView struct {
	model    *tui.Model
	viewport viewport.Model
	ready    bool
}

func NewSubscriptionView(model *tui.Model) *SubscriptionView {
	return &SubscriptionView{model: model}
}

func (v *SubscriptionView) Name() string { return "subscription" }

func (v *SubscriptionView) HasInline() bool { return false }

func (v *SubscriptionView) Init() tea.Cmd {
	content := v.renderAllLinks()
	w := v.model.ContentWidth()
	h := v.model.Height() - 6
	v.viewport = viewport.New(w, h)
	v.viewport.SetContent(content)
	v.ready = true
	return nil
}

func (v *SubscriptionView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.ViewResizeMsg:
		v.viewport.Width = msg.ContentWidth
		v.viewport.Height = msg.ContentHeight - 6
		return v, nil
	case tui.SubSplitMouseMsg:
		if msg.Button == tea.MouseButtonWheelUp {
			v.viewport.LineUp(3)
			return v, nil
		}
		if msg.Button == tea.MouseButtonWheelDown {
			v.viewport.LineDown(3)
			return v, nil
		}
		return v, nil
	case tea.KeyMsg:
		if msg.Type == tea.KeyEsc {
			return v, tui.BackCmd
		}
	}
	if v.ready {
		var cmd tea.Cmd
		v.viewport, cmd = v.viewport.Update(msg)
		return v, cmd
	}
	return v, nil
}

func (v *SubscriptionView) View() string {
	if !v.ready {
		return ""
	}
	return v.viewport.View()
}

func (v *SubscriptionView) renderAllLinks() string {
	s := v.model.Store()
	names := derived.UserNames(s)
	if len(names) == 0 {
		return "暂无用户"
	}

	headerStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
	userStyle := lipgloss.NewStyle().Foreground(tui.ColorAccent).Bold(true)
	divider := strings.Repeat("─", 68)

	var sb strings.Builder

	// Section: Protocol links (URI format).
	sb.WriteString(headerStyle.Render("[ 协议链接 ]"))
	sb.WriteString("\n")
	sb.WriteString(divider)
	sb.WriteString("\n")

	hasLinks := false
	for _, name := range names {
		links := subscription.Render(s, name, subscription.FormatURI, "")
		if len(links) == 0 {
			continue
		}
		hasLinks = true
		sb.WriteString(userStyle.Render(name))
		sb.WriteString("\n")
		for _, l := range links {
			sb.WriteString(fmt.Sprintf("  %s\n", l.Content))
		}
	}
	if !hasLinks {
		sb.WriteString("暂无可用链接\n")
	}

	sb.WriteString("\n")

	// Section: Surge links.
	sb.WriteString(headerStyle.Render("[ Surge 链接 ]"))
	sb.WriteString("\n")
	sb.WriteString(divider)
	sb.WriteString("\n")

	hasSurgeLinks := false
	for _, name := range names {
		links := subscription.Render(s, name, subscription.FormatSurge, "")
		if len(links) == 0 {
			continue
		}
		hasSurgeLinks = true
		sb.WriteString(userStyle.Render(name))
		sb.WriteString("\n")
		for _, l := range links {
			sb.WriteString(fmt.Sprintf("  %s\n", l.Content))
		}
	}
	if !hasSurgeLinks {
		sb.WriteString("暂无可用 Surge 链接\n")
	}

	return sb.String()
}
