package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/derived"
	"go-proxy/internal/subscription"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type SubscriptionView struct {
	model *tui.Model
}

func NewSubscriptionView(model *tui.Model) *SubscriptionView {
	return &SubscriptionView{model: model}
}

func (v *SubscriptionView) Name() string { return "subscription" }

func (v *SubscriptionView) Init() tea.Cmd {
	result := v.renderAllLinks()
	return func() tea.Msg {
		return tui.ShowOverlayMsg{
			Overlay: components.NewResult(result),
		}
	}
}

func (v *SubscriptionView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg.(type) {
	case tui.ResultDismissedMsg:
		return v, tui.BackCmd
	}
	return v, nil
}

func (v *SubscriptionView) View() string { return "" }

// renderAllLinks generates all subscription links for all users at once,
// organized by section like shell-proxy.
func (v *SubscriptionView) renderAllLinks() string {
	s := v.model.Store()
	names := derived.UserNames(s)
	if len(names) == 0 {
		return "暂无用户"
	}

	headerStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
	userStyle := lipgloss.NewStyle().Foreground(tui.ColorAccent).Bold(true)

	var sb strings.Builder

	// Section: Protocol links (Surge format).
	sb.WriteString(headerStyle.Render("[ 协议链接 ]"))
	sb.WriteString("\n\n")

	hasLinks := false
	for _, name := range names {
		links := subscription.Render(s, name, subscription.FormatSurge, "")
		if len(links) == 0 {
			continue
		}
		hasLinks = true
		sb.WriteString(userStyle.Render(fmt.Sprintf("  %s:", name)))
		sb.WriteString("\n")
		for _, l := range links {
			sb.WriteString(fmt.Sprintf("    %s\n", l.Content))
		}
		sb.WriteString("\n")
	}
	if !hasLinks {
		sb.WriteString("  暂无可用链接\n\n")
	}

	// Section: Snell info (if snell is configured).
	if s.SnellConf != nil {
		sb.WriteString(headerStyle.Render("[ Snell ]"))
		sb.WriteString("\n\n")
		port := snellPort(s.SnellConf.Listen)
		host := subscription.DetectTarget()
		sb.WriteString(fmt.Sprintf("  服务器: %s\n", subscription.FormatHost(host)))
		sb.WriteString(fmt.Sprintf("  端口: %d\n", port))
		sb.WriteString(fmt.Sprintf("  PSK: %s\n", s.SnellConf.PSK))
		sb.WriteString("\n")
	}

	// Section: Server info.
	sb.WriteString(headerStyle.Render("[ 服务器信息 ]"))
	sb.WriteString("\n\n")
	host := subscription.DetectTarget()
	sb.WriteString(fmt.Sprintf("  地址: %s\n", host))
	sb.WriteString(fmt.Sprintf("  用户数: %d\n", len(names)))
	sb.WriteString(fmt.Sprintf("  协议数: %d\n", len(s.SingBox.Inbounds)))

	return sb.String()
}
