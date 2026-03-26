package views

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/derived"
	"go-proxy/internal/subscription"
	"go-proxy/internal/tui"
)

// clearCopyNoticeMsg is sent after the copy feedback timer expires.
type clearCopyNoticeMsg struct{}

// userNameColors cycles through One Dark palette colors for per-user name display.
var userNameColors = []lipgloss.Color{
	tui.ColorBlue,       // #61AFEF
	tui.ColorError,      // #E06C75
	tui.ColorSuccess,    // #98C379
	tui.ColorDragBorder, // #E5C07B
	lipgloss.Color("#C678DD"),
	lipgloss.Color("#56B6C2"),
}

type SubscriptionView struct {
	model        *tui.Model
	viewport     viewport.Model
	ready        bool
	width        int
	target       string
	links        []string // all copyable link contents
	selectedLink int      // current selected link index (-1 = none)
	copyNotice   string   // temporary copy feedback
}

func NewSubscriptionView(model *tui.Model) *SubscriptionView {
	return &SubscriptionView{model: model, selectedLink: -1}
}

func (v *SubscriptionView) Name() string { return "subscription" }

func (v *SubscriptionView) HasInline() bool { return false }

func (v *SubscriptionView) Init() tea.Cmd {
	v.width = v.model.ContentWidth()
	v.target = subscription.DetectTarget()
	content := v.renderAllLinks()
	w := v.width
	h := v.model.Height() - 7
	v.viewport = viewport.New(w, h)
	v.viewport.SetContent(content)
	v.ready = true
	if len(v.links) > 0 {
		v.selectedLink = 0
	}
	return nil
}

func (v *SubscriptionView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.ViewResizeMsg:
		v.width = msg.ContentWidth
		v.viewport.Width = msg.ContentWidth
		v.viewport.Height = msg.ContentHeight - 5
		v.viewport.SetContent(v.renderAllLinks())
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
	case clearCopyNoticeMsg:
		v.copyNotice = ""
		v.viewport.SetContent(v.renderAllLinks())
		return v, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			return v, tui.BackCmd
		case tea.KeyUp:
			if len(v.links) > 0 {
				if v.selectedLink <= 0 {
					v.selectedLink = len(v.links) - 1
				} else {
					v.selectedLink--
				}
				v.viewport.SetContent(v.renderAllLinks())
			}
			return v, nil
		case tea.KeyDown:
			if len(v.links) > 0 {
				v.selectedLink = (v.selectedLink + 1) % len(v.links)
				v.viewport.SetContent(v.renderAllLinks())
			}
			return v, nil
		case tea.KeyCtrlC:
			if v.selectedLink >= 0 && v.selectedLink < len(v.links) {
				content := v.links[v.selectedLink]
				return v, v.doCopy(content)
			}
			return v, nil
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
	if v.copyNotice != "" {
		noticeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
		return v.viewport.View() + "\n" + noticeStyle.Render(v.copyNotice)
	}
	return v.viewport.View()
}

// doCopy copies content to clipboard (via atotto) and also emits OSC 52
// for SSH sessions. Sets copy notice and returns a timer cmd to clear it.
func (v *SubscriptionView) doCopy(content string) tea.Cmd {
	// Try system clipboard (works locally).
	_ = clipboard.WriteAll(content)
	v.copyNotice = "✓ 已复制"
	v.viewport.SetContent(v.renderAllLinks())
	// Emit OSC 52 sequence for SSH terminal clipboard and schedule clear.
	osc52 := osc52Cmd(content)
	clearCmd := tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return clearCopyNoticeMsg{}
	})
	return tea.Batch(osc52, clearCmd)
}

// osc52Cmd emits an OSC 52 escape sequence to copy content to the terminal
// clipboard. Uses direct stdout write to bypass bubbletea's alt-screen
// suppression. This works over SSH without X11 forwarding.
func osc52Cmd(content string) tea.Cmd {
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	seq := fmt.Sprintf("\x1b]52;c;%s\x1b\\", encoded)
	return func() tea.Msg {
		_, _ = os.Stdout.Write([]byte(seq))
		return nil
	}
}

// wrapLine hard-wraps s at maxWidth rune columns, returning lines joined by \n.
// It does not split ANSI escape sequences; use only on plain text.
func wrapLine(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return s
	}
	var out strings.Builder
	col := 0
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		s = s[size:]
		if r == '\n' {
			out.WriteRune(r)
			col = 0
			continue
		}
		if col >= maxWidth {
			out.WriteByte('\n')
			col = 0
		}
		out.WriteRune(r)
		col++
	}
	return out.String()
}

func (v *SubscriptionView) renderAllLinks() string {
	// Reset link index each render.
	v.links = nil

	s := v.model.Store()
	names := derived.UserNames(s)
	if len(names) == 0 {
		return "暂无用户"
	}

	w := v.width
	if w < 10 {
		w = 68
	}
	divider := strings.Repeat("─", w-2)

	headerStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
	highlightStyle := lipgloss.NewStyle().Foreground(tui.ColorFooterKey).Bold(true)

	userColorMap := make(map[string]lipgloss.Color, len(names))
	for i, name := range names {
		userColorMap[name] = userNameColors[i%len(userNameColors)]
	}

	var sb strings.Builder

	renderSection := func(format subscription.Format, header string) {
		sb.WriteString(headerStyle.Render(header))
		sb.WriteString("\n")
		sb.WriteString(divider)
		sb.WriteString("\n")

		hasLinks := false
		for _, name := range names {
			links := subscription.Render(s, name, format, v.target)
			if len(links) == 0 {
				continue
			}
			hasLinks = true
			uStyle := lipgloss.NewStyle().Foreground(userColorMap[name]).Bold(true)
			sb.WriteString(uStyle.Render(name + ":"))
			sb.WriteString("\n")
			for i, l := range links {
				idx := len(v.links)
				v.links = append(v.links, l.Content)
				content := wrapLine(l.Content, w-4)
				if idx == v.selectedLink {
					sb.WriteString("  ")
					sb.WriteString(highlightStyle.Render(content))
					sb.WriteString("\n")
				} else {
					sb.WriteString(fmt.Sprintf("  %s\n", content))
				}
				if i < len(links)-1 {
					sb.WriteString("\n")
				}
			}
		}
		if !hasLinks {
			sb.WriteString("暂无可用链接\n")
		}
		sb.WriteString("\n")
	}

	renderSection(subscription.FormatSurge, "[ Surge 链接 ]")
	renderSection(subscription.FormatURI, "[ 协议链接 ]")

	// Clamp selectedLink after rebuild.
	if v.selectedLink >= len(v.links) {
		v.selectedLink = len(v.links) - 1
	}

	return sb.String()
}
