package views

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/derived"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
	"go-proxy/internal/user"
)

type UserView struct {
	tui.SplitViewBase
	step    userStep
	oldName string // for rename: stores the old username
	rows    []userListRow
}

type userStep int

const (
	userMenu userStep = iota
	userList
	userAdd
	userRenameOld
	userRenameNew
	userDelete
	userResult
)

type userListRow struct {
	Name     string
	Protocol string
}

func NewUserView(model *tui.Model) *UserView {
	v := &UserView{}
	v.Model = model
	v.Menu = tui.NewMenu("", []tui.MenuItem{
		{Key: '1', Label: "󰋼 用户列表", ID: "list"},
		{Key: '2', Label: "󰐕 添加用户", ID: "add"},
		{Key: '3', Label: "󰑕 重置用户", ID: "rename"},
		{Key: '4', Label: "󰍷 删除用户", ID: "delete"},
	})
	cw := model.ContentWidth()
	v.Split = tui.NewSubSplit(cw, model.Height()-5)
	return v
}

func (v *UserView) Name() string { return "user" }

func (v *UserView) Init() tea.Cmd {
	v.step = userMenu
	v.Split.SetFocusLeft(true)
	return nil
}

func (v *UserView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.ViewResizeMsg:
		v.HandleResize(msg)
		return v, nil

	case tui.SubSplitMouseMsg:
		return v, v.HandleMouse(msg)
	}

	// In split mode, intercept up/down for menu navigation even when content is showing.
	if cmd, handled := v.HandleMenuNav(msg, v.step == userMenu, false); handled {
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
		v.SetFocus(false)
		return v, v.triggerMenuAction(msg.ID)

	case tui.InputResultMsg:
		if msg.Cancelled {
			v.step = userMenu
			v.SetFocus(true)
			return v, nil
		}
		switch v.step {
		case userAdd:
			val := msg.Value
			return v, func() tea.Msg { return v.doAdd(val) }
		case userRenameOld:
			v.oldName = msg.Value
			v.step = userRenameNew
			return v, v.SetInline(components.NewTextInput("新用户名:", ""))
		case userRenameNew:
			oldName := v.oldName
			newName := msg.Value
			return v, func() tea.Msg { return v.doRename(oldName, newName) }
		case userDelete:
			val := msg.Value
			return v, func() tea.Msg { return v.doDelete(val) }
		}

	case userActionDoneMsg:
		v.step = userResult
		return v, v.SetInline(components.NewResult(msg.result))

	case tui.ResultDismissedMsg:
		v.step = userMenu
		v.SetFocus(true)
		return v, nil

	default:
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if keyMsg.Type == tea.KeyEsc {
				if v.step != userMenu {
					v.step = userMenu
					v.SetFocus(true)
					return v, nil
				}
				return v, tui.BackCmd
			}
			// Left/Right arrow toggles sub-split focus.
			if v.HandleSplitArrows(keyMsg, v.step == userMenu, v.HasInline()) {
				return v, nil
			}
		}
		if v.step == userMenu {
			var cmd tea.Cmd
			v.Menu, cmd = v.Menu.Update(msg)
			return v, cmd
		}
	}
	return v, inlineCmd
}

// triggerMenuAction executes the action for the given menu item ID.
func (v *UserView) triggerMenuAction(id string) tea.Cmd {
	switch id {
	case "list":
		v.step = userList
		return v.listUsers
	case "add":
		v.step = userAdd
		return v.SetInline(components.NewTextInput("输入用户名:", ""))
	case "rename":
		names := derived.UserNames(v.Model.Store())
		if len(names) == 0 {
			v.step = userResult
			return v.SetInline(components.NewResult("暂无用户"))
		}
		v.step = userRenameOld
		return v.SetInline(components.NewSelectList("选择要重命名的用户:", names))
	case "delete":
		names := derived.UserNames(v.Model.Store())
		if len(names) == 0 {
			v.step = userResult
			return v.SetInline(components.NewResult("暂无用户"))
		}
		v.step = userDelete
		return v.SetInline(components.NewSelectList("选择要删除的用户:", names))
	}
	return nil
}

func (v *UserView) View() string {
	// 未选操作或分栏不可用：保持原行为
	if v.step == userMenu || !v.Split.Enabled() {
		if v.HasInline() {
			return v.ViewInline()
		}
		return tui.RenderSubMenuBody(v.Menu.View(), v.Split.TotalWidth())
	}

	// Sub-split: left = menu, right = inline component or hint.
	menuContent := v.Menu.View()

	var detailContent string
	if v.HasInline() {
		tui.InSplitPanel = true
		detailContent = v.ViewInline()
		tui.InSplitPanel = false
	} else {
		if v.step == userList {
			detailContent = v.renderUserListTable()
		} else {
			detailContent = lipgloss.NewStyle().
				Foreground(tui.ColorMuted).
				Render("加载中...")
		}
	}

	return v.Split.View(menuContent, detailContent)
}

type userActionDoneMsg struct{ result string }

func (v *UserView) listUsers() tea.Msg {
	users := user.List(v.Model.Store())
	v.rows = nil
	nameWidth := lipgloss.Width("用户列表")
	for _, u := range users {
		if w := lipgloss.Width(u.Name); w > nameWidth {
			nameWidth = w
		}
	}
	for _, u := range users {
		protoSummary := "(无协议)"
		if len(u.Memberships) > 0 {
			seen := make(map[string]bool)
			for _, m := range u.Memberships {
				seen[m.Proto] = true
			}
			var parts []string
			for proto := range seen {
				parts = append(parts, proto)
			}
			sort.Strings(parts)
			protoSummary = strings.Join(parts, ", ")
		}
		v.rows = append(v.rows, userListRow{
			Name:     u.Name,
			Protocol: protoSummary,
		})
	}
	return userActionDoneMsg{result: v.renderUserListTable()}
}

func (v *UserView) doAdd(name string) tea.Msg {
	if err := user.Add(v.Model.Store(), name); err != nil {
		return userActionDoneMsg{result: "失败: " + err.Error()}
	}
	if err := v.Model.Store().Apply(); err != nil {
		return userActionDoneMsg{result: "失败: " + err.Error()}
	}
	return userActionDoneMsg{result: "已添加用户: " + name}
}

func (v *UserView) doRename(oldName, newName string) tea.Msg {
	if err := user.Rename(v.Model.Store(), oldName, newName); err != nil {
		return userActionDoneMsg{result: "失败: " + err.Error()}
	}
	if err := v.Model.Store().Apply(); err != nil {
		return userActionDoneMsg{result: "失败: " + err.Error()}
	}
	return userActionDoneMsg{result: fmt.Sprintf("已重置 %s → %s", oldName, newName)}
}

func (v *UserView) doDelete(name string) tea.Msg {
	if err := user.Delete(v.Model.Store(), name); err != nil {
		return userActionDoneMsg{result: "失败: " + err.Error()}
	}
	if err := v.Model.Store().Apply(); err != nil {
		return userActionDoneMsg{result: "失败: " + err.Error()}
	}
	return userActionDoneMsg{result: "已删除用户: " + name}
}

func padDisplayCell(text string, width int) string {
	padding := width - lipgloss.Width(text)
	if padding < 0 {
		padding = 0
	}
	return text + strings.Repeat(" ", padding)
}

func (v *UserView) renderUserListTable() string {
	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorLabel).Bold(true)
	nameStyle := lipgloss.NewStyle().Foreground(tui.ColorBlack).Bold(true)
	valStyle := lipgloss.NewStyle().Foreground(tui.ColorBlack)
	sepStyle := lipgloss.NewStyle().Foreground(tui.ColorMuted)

	nameWidth := lipgloss.Width("用户列表")
	for _, row := range v.rows {
		if w := lipgloss.Width(row.Name); w > nameWidth {
			nameWidth = w
		}
	}
	nameWidth += 4

	var sb strings.Builder
	sb.WriteString("  ")
	sb.WriteString(labelStyle.Render(padDisplayCell("用户列表", nameWidth)))
	sb.WriteString(labelStyle.Render("协议"))
	sb.WriteString("\n")
	sb.WriteString("  ")
	sb.WriteString(sepStyle.Render(strings.Repeat("─", nameWidth+18)))
	sb.WriteString("\n")

	if len(v.rows) == 0 {
		sb.WriteString("  暂无用户\n")
		return sb.String()
	}

	for _, row := range v.rows {
		sb.WriteString("  ")
		sb.WriteString(nameStyle.Render(padDisplayCell(row.Name, nameWidth)))
		sb.WriteString(valStyle.Render(row.Protocol))
		sb.WriteString("\n")
	}
	return sb.String()
}
