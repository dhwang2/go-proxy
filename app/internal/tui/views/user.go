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
	tui.InlineState
	model   *tui.Model
	menu    tui.MenuModel
	split   tui.SubSplitModel
	step    userStep
	oldName string // for rename: stores the old username
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

func NewUserView(model *tui.Model) *UserView {
	v := &UserView{model: model}
	v.menu = tui.NewMenu("", []tui.MenuItem{
		{Key: '1', Label: "󰋼 用户列表", ID: "list"},
		{Key: '2', Label: "󰐕 添加用户", ID: "add"},
		{Key: '3', Label: "󰑕 重置用户", ID: "rename"},
		{Key: '4', Label: "󰍷 删除用户", ID: "delete"},
	})
	cw := model.ContentWidth()
	v.split = tui.NewSubSplit(cw, model.Height()-6)
	return v
}

func (v *UserView) Name() string { return "user" }

func (v *UserView) Init() tea.Cmd {
	v.step = userMenu
	v.split.SetFocusLeft(true)
	return nil
}

func (v *UserView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
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
	if v.split.Enabled() && v.step != userMenu {
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
		// Auto-preview: trigger action without changing focus.
		return v, v.triggerMenuAction(msg.ID)

	case tui.MenuSelectMsg:
		v.split.SetFocusLeft(false)
		return v, v.triggerMenuAction(msg.ID)

	case tui.InputResultMsg:
		if msg.Cancelled {
			v.step = userMenu
			v.split.SetFocusLeft(true)
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
		v.split.SetFocusLeft(true)
		return v, nil

	default:
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if keyMsg.Type == tea.KeyEsc {
				if v.step != userMenu {
					v.step = userMenu
					v.split.SetFocusLeft(true)
					return v, nil
				}
				return v, tui.BackCmd
			}
			// Left/Right arrow toggles sub-split focus.
			if v.split.Enabled() && v.step != userMenu {
				if keyMsg.Type == tea.KeyLeft {
					v.split.SetFocusLeft(true)
					return v, nil
				}
				if keyMsg.Type == tea.KeyRight {
					v.split.SetFocusLeft(false)
					return v, nil
				}
			}
		}
		if v.step == userMenu {
			var cmd tea.Cmd
			v.menu, cmd = v.menu.Update(msg)
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
		names := derived.UserNames(v.model.Store())
		if len(names) == 0 {
			v.step = userResult
			return v.SetInline(components.NewResult("暂无用户"))
		}
		v.step = userRenameOld
		return v.SetInline(components.NewSelectList("选择要重命名的用户:", names))
	case "delete":
		names := derived.UserNames(v.model.Store())
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
	if v.step == userMenu || !v.split.Enabled() {
		if v.HasInline() {
			return v.ViewInline()
		}
		return tui.RenderSubMenuBody(v.menu.View(), v.split.TotalWidth())
	}

	// Sub-split: left = menu, right = inline component or hint.
	menuContent := v.menu.View()

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

type userActionDoneMsg struct{ result string }

func (v *UserView) listUsers() tea.Msg {
	users := user.List(v.model.Store())
	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorLabel).Bold(true)
	valStyle := lipgloss.NewStyle().Foreground(tui.ColorValSys)
	sepStyle := lipgloss.NewStyle().Foreground(tui.ColorMuted)

	var sb strings.Builder
	// Find max name width for alignment.
	nameWidth := 8 // minimum
	for _, u := range users {
		if w := lipgloss.Width(u.Name); w > nameWidth {
			nameWidth = w
		}
	}
	nameWidth += 2 // padding

	sb.WriteString(labelStyle.Render(fmt.Sprintf("%-*s", nameWidth, "用户列表")))
	sb.WriteString(labelStyle.Render("协议"))
	sb.WriteString("\n")
	sb.WriteString(sepStyle.Render(strings.Repeat("─", 42)))
	sb.WriteString("\n")

	if len(users) == 0 {
		sb.WriteString("暂无用户\n")
	}
	for _, u := range users {
		protoSummary := "(无协议)"
		if len(u.Memberships) > 0 {
			counts := make(map[string]int)
			for _, m := range u.Memberships {
				counts[m.Proto]++
			}
			var parts []string
			for proto, count := range counts {
				parts = append(parts, fmt.Sprintf("%s x%d", proto, count))
			}
			sort.Strings(parts)
			protoSummary = strings.Join(parts, ", ")
		}
		sb.WriteString(labelStyle.Render(fmt.Sprintf("%-*s", nameWidth, u.Name)))
		sb.WriteString(valStyle.Render(protoSummary))
		sb.WriteString("\n")
	}
	return userActionDoneMsg{result: sb.String()}
}

func (v *UserView) doAdd(name string) tea.Msg {
	if err := user.Add(v.model.Store(), name); err != nil {
		return userActionDoneMsg{result: "失败: " + err.Error()}
	}
	if err := v.model.Store().Apply(); err != nil {
		return userActionDoneMsg{result: "失败: " + err.Error()}
	}
	return userActionDoneMsg{result: "已添加用户: " + name}
}

func (v *UserView) doRename(oldName, newName string) tea.Msg {
	if err := user.Rename(v.model.Store(), oldName, newName); err != nil {
		return userActionDoneMsg{result: "失败: " + err.Error()}
	}
	if err := v.model.Store().Apply(); err != nil {
		return userActionDoneMsg{result: "失败: " + err.Error()}
	}
	return userActionDoneMsg{result: fmt.Sprintf("已重置 %s → %s", oldName, newName)}
}

func (v *UserView) doDelete(name string) tea.Msg {
	if err := user.Delete(v.model.Store(), name); err != nil {
		return userActionDoneMsg{result: "失败: " + err.Error()}
	}
	if err := v.model.Store().Apply(); err != nil {
		return userActionDoneMsg{result: "失败: " + err.Error()}
	}
	return userActionDoneMsg{result: "已删除用户: " + name}
}
