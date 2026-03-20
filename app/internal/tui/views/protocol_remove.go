package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/derived"
	"go-proxy/internal/protocol"
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
}

type protoRemoveStep int

const (
	protoRemoveMenu protoRemoveStep = iota
	protoRemoveConfirm
	protoRemoveResult
)

func NewProtocolRemoveView(model *tui.Model) *ProtocolRemoveView {
	return &ProtocolRemoveView{model: model}
}

func (v *ProtocolRemoveView) Name() string { return "protocol-remove" }

func (v *ProtocolRemoveView) Init() tea.Cmd {
	v.step = protoRemoveMenu
	v.pendingTag = ""
	v.emptyResult = false
	v.split.SetFocusLeft(true)
	v.split.SetSize(v.model.ContentWidth(), v.model.Height()-6)
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

		label := fmt.Sprintf("%-14s %-8d %s",
			info.Type, info.Port, userNames)

		items = append(items, tui.MenuItem{
			Key:   k,
			Label: label,
			ID:    info.Tag,
		})
	}
	v.menu = v.menu.SetItems(items)
	return nil
}

func (v *ProtocolRemoveView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.ViewResizeMsg:
		v.split.SetSize(msg.ContentWidth, msg.ContentHeight-6)
		return v, nil
	case tui.SubSplitMouseMsg:
		var cmd tea.Cmd
		v.split, cmd = v.split.Update(msg.MouseMsg)
		return v, cmd
	}
	// In split mode, intercept up/down for menu navigation even when content is showing.
	if v.split.Enabled() && v.step != protoRemoveMenu {
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
		return v, v.triggerMenuAction(msg.ID)
	case tui.MenuSelectMsg:
		v.split.SetFocusLeft(false)
		return v, v.triggerMenuAction(msg.ID)

	case tui.ConfirmResultMsg:
		if !msg.Confirmed {
			v.step = protoRemoveMenu
			v.split.SetFocusLeft(true)
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
		if v.step == protoRemoveMenu {
			var cmd tea.Cmd
			v.menu, cmd = v.menu.Update(msg)
			return v, cmd
		}
	}
	return v, inlineCmd
}

func (v *ProtocolRemoveView) View() string {
	if v.step == protoRemoveMenu || !v.split.Enabled() {
		if v.HasInline() {
			return v.ViewInline()
		}
		if v.step == protoRemoveMenu && v.tableHeader != "" {
			content := v.tableHeader + "\n" + v.menu.View()
			return tui.RenderSubMenuBody(content, v.model.ContentWidth())
		}
		return tui.RenderSubMenuBody(v.menu.View(), v.model.ContentWidth())
	}

	var menuContent string
	if v.tableHeader != "" {
		menuContent = v.tableHeader + "\n" + v.menu.View()
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
	if derived.FindInbound(v.model.Store(), id) == nil {
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
