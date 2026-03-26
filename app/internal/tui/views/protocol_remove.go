package views

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/derived"
	"go-proxy/internal/protocol"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type ProtocolRemoveView struct {
	tui.SplitViewBase
	step        protoRemoveStep
	pendingTag  string
	pendingUser string // specific user to remove (empty = remove all)
	tagUsers    map[string][]string
	tableHeader string
	emptyResult bool
	rows        []protocolRemoveRow
	userMenu    tui.MenuModel
}

type protoRemoveStep int

const (
	protoRemoveMenu protoRemoveStep = iota
	protoRemoveUserSelect
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
	v := &ProtocolRemoveView{}
	v.Model = model
	return v
}

func (v *ProtocolRemoveView) Name() string { return "protocol-remove" }

func (v *ProtocolRemoveView) setFocus(left bool) {
	v.SetFocus(left, func(l bool) {
		v.userMenu = v.userMenu.SetDim(l)
	})
}

func (v *ProtocolRemoveView) Init() tea.Cmd {
	v.step = protoRemoveMenu
	v.pendingTag = ""
	v.pendingUser = ""
	v.emptyResult = false
	v.rows = nil
	v.tableHeader = ""
	v.Menu = v.Menu.SetItems(nil)
	v.userMenu = tui.MenuModel{}
	v.ClearInline()
	v.InitSplit()
	v.Split.SetMinWidths(14, 10)
	v.Model.Store().Reload()
	inv := derived.Inventory(v.Model.Store())

	if len(inv) == 0 {
		v.step = protoRemoveResult
		v.emptyResult = true
		return v.SetInline(components.NewResult("没有已安装的协议"))
	}

	membership := derived.Membership(v.Model.Store())

	// Build reverse map: tag -> list of user names.
	v.tagUsers = make(map[string][]string)
	for name, entries := range membership {
		for _, e := range entries {
			v.tagUsers[e.Tag] = append(v.tagUsers[e.Tag], name)
		}
	}

	// Table header + separator.
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

		userNames := strings.Join(v.tagUsers[info.Tag], "  ")
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
	v.Menu = v.Menu.SetItems(items)
	return nil
}

func (v *ProtocolRemoveView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.ViewResizeMsg:
		v.HandleResize(msg)
		return v, nil
	case tui.SubSplitMouseMsg:
		return v, v.HandleMouse(msg)
	}
	if cmd, handled := v.HandleMenuNav(msg, v.step == protoRemoveMenu, false); handled {
		return v, cmd
	}
	inlineCmd, handled := v.UpdateInline(msg)
	if handled {
		return v, inlineCmd
	}
	switch msg := msg.(type) {
	case tui.MenuCursorChangeMsg:
		return v, nil

	case tui.MenuSelectMsg:
		// Check if this is a user selection in the user sub-menu.
		if v.step == protoRemoveUserSelect && !(v.Split.Enabled() && v.Split.FocusLeft()) {
			v.pendingUser = msg.ID
			v.step = protoRemoveConfirm
			prompt := fmt.Sprintf("确认从 %s 卸载用户 %s?", v.pendingTag, v.pendingUser)
			v.setFocus(false)
			return v, v.SetInline(components.NewConfirm(prompt))
		}
		v.setFocus(false)
		return v, v.triggerMenuAction(msg.ID)

	case tui.ConfirmResultMsg:
		if !msg.Confirmed {
			v.step = protoRemoveMenu
			v.pendingTag = ""
			v.pendingUser = ""
			v.ClearInline()
			v.setFocus(true)
			return v, nil
		}
		tag := v.pendingTag
		userName := v.pendingUser
		return v, tea.Batch(
			v.SetInline(components.NewSpinner("正在卸载...")),
			func() tea.Msg {
				return protoRemoveDoneMsg{tag: tag, user: userName, err: v.doRemove(tag, userName)}
			},
		)

	case protoRemoveDoneMsg:
		v.step = protoRemoveResult
		var result string
		if msg.err != nil {
			result = "卸载失败: " + msg.err.Error()
		} else if msg.user != "" {
			result = fmt.Sprintf("已从 %s 卸载用户: %s", msg.tag, msg.user)
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
			switch v.step {
			case protoRemoveUserSelect:
				v.step = protoRemoveMenu
				v.pendingTag = ""
				v.setFocus(true)
				return v, nil
			default:
				return v, tui.BackCmd
			}
		}
		// Left/Right arrow toggles sub-split focus.
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if v.Split.Enabled() && v.step != protoRemoveMenu {
				if keyMsg.Type == tea.KeyLeft {
					v.setFocus(true)
					return v, nil
				}
				if keyMsg.Type == tea.KeyRight && (v.HasInline() || v.step == protoRemoveUserSelect) {
					v.setFocus(false)
					return v, nil
				}
			}
		}
		switch v.step {
		case protoRemoveMenu:
			var cmd tea.Cmd
			v.Menu, cmd = v.Menu.Update(msg)
			return v, cmd
		case protoRemoveUserSelect:
			var cmd tea.Cmd
			if v.Split.Enabled() && v.Split.FocusLeft() {
				v.Menu, cmd = v.Menu.Update(msg)
			} else {
				v.userMenu, cmd = v.userMenu.Update(msg)
			}
			return v, cmd
		}
	}
	return v, inlineCmd
}

func (v *ProtocolRemoveView) View() string {
	if v.emptyResult && v.HasInline() {
		return v.ViewInline()
	}

	// Non-split fallback.
	if !v.Split.Enabled() {
		if v.HasInline() {
			return v.ViewInline()
		}
		if v.step == protoRemoveUserSelect {
			return tui.RenderSubMenuBody(v.userMenu.View(), v.Model.ContentWidth())
		}
		if v.step == protoRemoveMenu && v.tableHeader != "" {
			return tui.RenderSubMenuBody(v.renderRemoveTable(), v.Model.ContentWidth())
		}
		return tui.RenderSubMenuBody(v.Menu.View(), v.Model.ContentWidth())
	}

	// Split mode: main menu step has no split content.
	if v.step == protoRemoveMenu {
		if v.tableHeader != "" {
			return tui.RenderSubMenuBody(v.renderRemoveTable(), v.Model.ContentWidth())
		}
		return tui.RenderSubMenuBody(v.Menu.View(), v.Model.ContentWidth())
	}

	// LEFT: always table/menu.
	var menuContent string
	if v.tableHeader != "" {
		menuContent = v.renderRemoveTable()
	} else {
		menuContent = v.Menu.View()
	}

	// RIGHT: user menu, inline, or empty.
	var detailContent string
	if v.HasInline() {
		tui.InSplitPanel = true
		detailContent = v.ViewInline()
		tui.InSplitPanel = false
	} else if v.step == protoRemoveUserSelect {
		detailContent = v.userMenu.View()
	} else {
		detailContent = lipgloss.NewStyle().
			Foreground(tui.ColorMuted).
			Render("加载中...")
	}

	return v.Split.View(menuContent, detailContent)
}

// triggerMenuAction executes the action for the given menu item ID.
func (v *ProtocolRemoveView) triggerMenuAction(id string) tea.Cmd {
	if id != store.SnellTag && derived.FindInbound(v.Model.Store(), id) == nil {
		return nil
	}
	v.pendingTag = id

	users := v.tagUsers[id]

	// Single user or snell: go directly to confirm.
	if len(users) <= 1 || id == store.SnellTag {
		v.pendingUser = ""
		v.step = protoRemoveConfirm
		prompt := fmt.Sprintf("确认卸载 %s?", v.pendingTag)
		return v.SetInline(components.NewConfirm(prompt))
	}

	// Multiple users: show user selection menu.
	v.step = protoRemoveUserSelect
	items := make([]tui.MenuItem, len(users))
	for i, name := range users {
		k := rune('1' + i)
		if i >= 9 {
			k = rune('a' + i - 9)
		}
		items[i] = tui.MenuItem{Key: k, Label: name, ID: name}
	}
	v.userMenu = tui.NewMenu("选择要卸载的用户:", items)
	return nil
}

type protoRemoveDoneMsg struct {
	tag  string
	user string
	err  error
}

type shadowTLSCleanupTarget struct {
	backendProto string
	backendPort  int
}

func (v *ProtocolRemoveView) doRemove(tag, userName string) error {
	cleanup := v.shadowTLSCleanupTarget(tag, userName)
	var err error
	if userName != "" {
		err = protocol.RemoveUserFromInbound(v.Model.Store(), tag, userName)
	} else {
		err = protocol.Remove(v.Model.Store(), tag)
	}
	if err != nil {
		return err
	}
	if err := v.Model.Store().Apply(); err != nil {
		return err
	}
	if cleanup != nil {
		if err := service.RemoveShadowTLSBindingByBackend(context.Background(), cleanup.backendProto, cleanup.backendPort); err != nil {
			return err
		}
	}
	return nil
}

func (v *ProtocolRemoveView) shadowTLSCleanupTarget(tag, userName string) *shadowTLSCleanupTarget {
	if tag == store.SnellTag {
		if v.Model.Store().SnellConf == nil {
			return nil
		}
		return &shadowTLSCleanupTarget{backendProto: "snell", backendPort: v.Model.Store().SnellConf.Port()}
	}

	ib := derived.FindInbound(v.Model.Store(), tag)
	if ib == nil || ib.Type != "shadowsocks" || ib.ListenPort == 0 {
		return nil
	}
	if userName == "" || len(ib.Users) == 0 {
		return &shadowTLSCleanupTarget{backendProto: "ss", backendPort: ib.ListenPort}
	}
	if len(ib.Users) == 1 && ib.Users[0].Name == userName {
		return &shadowTLSCleanupTarget{backendProto: "ss", backendPort: ib.ListenPort}
	}
	return nil
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
	valStyle := lipgloss.NewStyle().Foreground(tui.ColorBlack)
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
		if i == v.Menu.Cursor() && !v.Menu.IsDimmed() {
			sb.WriteString(selectedStyle.Render(line))
		} else {
			sb.WriteString(valStyle.Render(line))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
