package views

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/derived"
	"go-proxy/internal/protocol"
	"go-proxy/internal/store"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type ProtocolRemoveView struct {
	tui.InlineState
	model       *tui.Model
	menu        tui.MenuModel
	split       tui.SubSplitModel
	step        protoRemoveStep
	pendingTag  string
	tableHeader string
	emptyResult bool
	rows        []protocolRemoveRow
}

type protoRemoveStep int

const (
	protoRemoveMenu protoRemoveStep = iota
	protoRemoveConfirm
	protoRemoveResult
)

type protocolRemoveRow struct {
	Key      rune
	ID       string
	Protocol string
	Port     string
	User     string
}

func NewProtocolRemoveView(model *tui.Model) *ProtocolRemoveView {
	return &ProtocolRemoveView{model: model}
}

func (v *ProtocolRemoveView) Name() string { return "protocol-remove" }

func (v *ProtocolRemoveView) setFocus(left bool) {
	v.split.SetFocusLeft(left)
	v.menu = v.menu.SetDim(!left)
}

func (v *ProtocolRemoveView) Init() tea.Cmd {
	v.step = protoRemoveMenu
	v.pendingTag = ""
	v.emptyResult = false
	v.split.SetFocusLeft(true)
	v.split.SetMinWidths(14, 10)
	v.split.SetSize(v.model.ContentWidth(), v.model.Height()-5)
	// Reload store from disk to pick up changes from protocol install.
	v.model.Store().Reload()
	inv := derived.Inventory(v.model.Store())

	if len(inv) == 0 {
		v.step = protoRemoveResult
		v.emptyResult = true
		return v.SetInline(components.NewResult("没有已安装的协议"))
	}

	membership := derived.Membership(v.model.Store())

	// Build a reverse map: tag -> list of user names.
	tagUsers := make(map[string][]string)
	for name, entries := range membership {
		for _, e := range entries {
			tagUsers[e.Tag] = append(tagUsers[e.Tag], name)
		}
	}

	// Table header + separator.
	// Menu prefix is 7 display-cells ("   1.  "), label uses "%-14s %-8d %s".
	// CJK chars are 2 cells wide, so pad manually to match ASCII column widths.
	header := "  #    协议          端口     用户"
	sep := "  " + strings.Repeat("─", 50)
	v.tableHeader = header + "\n" + sep

	v.rows = nil
	items := make([]tui.MenuItem, 0, len(inv)+1)
	for i, info := range inv {
		k := rune('1' + i)
		if i >= 9 {
			k = rune('a' + i - 9)
		}

		userNames := strings.Join(tagUsers[info.Tag], "  ")
		if userNames == "" {
			userNames = "—"
		}

		v.rows = append(v.rows, protocolRemoveRow{
			Key:      k,
			ID:       info.Tag,
			Protocol: info.Type,
			Port:     strconv.Itoa(info.Port),
			User:     userNames,
		})
		items = append(items, tui.MenuItem{
			Key:   k,
			Label: info.Type,
			ID:    info.Tag,
		})
	}
	v.menu = v.menu.SetItems(items)
	return nil
}

func (v *ProtocolRemoveView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.ViewResizeMsg:
		v.split.SetSize(msg.ContentWidth, msg.ContentHeight-5)
		return v, nil
	case tui.SubSplitMouseMsg:
		var cmd tea.Cmd
		v.split, cmd = v.split.Update(msg.MouseMsg)
		return v, cmd
	}
	// In split mode, intercept up/down for menu navigation even when content is showing.
	if v.split.Enabled() && v.step != protoRemoveMenu && v.split.FocusLeft() {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if keyMsg.Type == tea.KeyUp || keyMsg.Type == tea.KeyDown {
				var cmd tea.Cmd
				v.menu, cmd = v.menu.Update(msg)
				return v, cmd
			}
		}
	}
	inlineCmd, handled := v.UpdateInline(msg)
	if handled {
		return v, inlineCmd
	}
	switch msg := msg.(type) {
	case tui.MenuCursorChangeMsg:
		// Do not auto-preview — triggerMenuAction starts the confirm dialog.
		return v, nil
	case tui.MenuSelectMsg:
		v.setFocus(false)
		return v, v.triggerMenuAction(msg.ID)

	case tui.ConfirmResultMsg:
		if !msg.Confirmed {
			v.step = protoRemoveMenu
			v.setFocus(true)
			return v, nil
		}
		tag := v.pendingTag
		return v, tea.Batch(
			v.SetInline(components.NewSpinner("正在卸载...")),
			func() tea.Msg {
				return protoRemoveDoneMsg{tag: tag, err: v.doRemove(tag)}
			},
		)

	case protoRemoveDoneMsg:
		v.step = protoRemoveResult
		var result string
		if msg.err != nil {
			result = "卸载失败: " + msg.err.Error()
		} else {
			result = "已卸载: " + msg.tag
		}
		return v, v.SetInline(components.NewResult(result))

	case tui.ResultDismissedMsg:
		if v.emptyResult {
			return v, tui.BackCmd
		}
		cmd := v.Init()
		return v, cmd

	default:
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyEsc {
			return v, tui.BackCmd
		}
		// Left/Right arrow toggles sub-split focus.
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if v.split.Enabled() && v.step != protoRemoveMenu {
				if keyMsg.Type == tea.KeyLeft {
					v.setFocus(true)
					return v, nil
				}
				if keyMsg.Type == tea.KeyRight && v.HasInline() {
					v.setFocus(false)
					return v, nil
				}
			}
		}
		if v.step == protoRemoveMenu {
			var cmd tea.Cmd
			v.menu, cmd = v.menu.Update(msg)
			return v, cmd
		}
	}
	return v, inlineCmd
}

func (v *ProtocolRemoveView) IsSubSplitRightFocused() bool {
	return v.split.Enabled() && !v.split.FocusLeft()
}

func (v *ProtocolRemoveView) View() string {
	if v.step == protoRemoveMenu || !v.split.Enabled() {
		if v.HasInline() {
			return v.ViewInline()
		}
		if v.step == protoRemoveMenu && v.tableHeader != "" {
			content := v.renderRemoveTable()
			return tui.RenderSubMenuBody(content, v.model.ContentWidth())
		}
		return tui.RenderSubMenuBody(v.menu.View(), v.model.ContentWidth())
	}

	var menuContent string
	if v.tableHeader != "" {
		menuContent = v.renderRemoveTable()
	} else {
		menuContent = v.menu.View()
	}

	var detailContent string
	if v.HasInline() {
		tui.InSplitPanel = true
		detailContent = v.ViewInline()
		tui.InSplitPanel = false
	} else {
		detailContent = lipgloss.NewStyle().
			Foreground(tui.ColorMuted).
			Render("加载中...")
	}

	return v.split.View(menuContent, detailContent)
}

// triggerMenuAction executes the action for the given menu item ID.
func (v *ProtocolRemoveView) triggerMenuAction(id string) tea.Cmd {
	if id != store.SnellTag && derived.FindInbound(v.model.Store(), id) == nil {
		return nil
	}
	v.pendingTag = id
	v.step = protoRemoveConfirm
	prompt := fmt.Sprintf("确认卸载 %s?", v.pendingTag)
	return v.SetInline(components.NewConfirm(prompt))
}

type protoRemoveDoneMsg struct {
	tag string
	err error
}

func (v *ProtocolRemoveView) doRemove(tag string) error {
	if err := protocol.Remove(v.model.Store(), tag); err != nil {
		return err
	}
	return v.model.Store().Apply()
}

func padProtocolRemoveCell(text string, width int) string {
	padding := width - lipgloss.Width(text)
	if padding < 0 {
		padding = 0
	}
	return text + strings.Repeat(" ", padding)
}

func (v *ProtocolRemoveView) renderRemoveTable() string {
	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorLabel).Bold(true)
	valStyle := lipgloss.NewStyle().Foreground(tui.ColorValSys)
	sepStyle := lipgloss.NewStyle().Foreground(tui.ColorMuted)
	selectedStyle := lipgloss.NewStyle().
		Background(tui.ColorAccent).
		Foreground(tui.ColorAccentFg).
		Bold(true)

	protocolWidth := lipgloss.Width("协议")
	portWidth := lipgloss.Width("端口")
	for _, row := range v.rows {
		if w := lipgloss.Width(row.Protocol); w > protocolWidth {
			protocolWidth = w
		}
		if w := lipgloss.Width(row.Port); w > portWidth {
			portWidth = w
		}
	}
	protocolWidth += 4
	portWidth += 4

	var sb strings.Builder
	sb.WriteString("  ")
	sb.WriteString(labelStyle.Render("#  "))
	sb.WriteString(labelStyle.Render(padProtocolRemoveCell("协议", protocolWidth)))
	sb.WriteString(labelStyle.Render(padProtocolRemoveCell("端口", portWidth)))
	sb.WriteString(labelStyle.Render("用户"))
	sb.WriteString("\n")
	sb.WriteString("  ")
	sb.WriteString(sepStyle.Render(strings.Repeat("─", protocolWidth+portWidth+18)))
	sb.WriteString("\n")

	for i, row := range v.rows {
		line := "  " + string(row.Key) + ". " +
			padProtocolRemoveCell(row.Protocol, protocolWidth) +
			padProtocolRemoveCell(row.Port, portWidth) +
			row.User
		if i == v.menu.Cursor() && !v.menu.IsDimmed() {
			sb.WriteString(selectedStyle.Render(line))
		} else {
			sb.WriteString(valStyle.Render(line))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
