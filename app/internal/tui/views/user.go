package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"go-proxy/internal/derived"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
	"go-proxy/internal/user"
)

type UserView struct {
	model   *tui.Model
	menu    components.MenuModel
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
	v.menu = components.NewMenu("󰁥 用户管理", []components.MenuItem{
		{Key: '1', Label: "󰋼 用户列表", ID: "list"},
		{Key: '2', Label: "󰐕 添加用户", ID: "add"},
		{Key: '3', Label: "󰑕 重置用户", ID: "rename"},
		{Key: '4', Label: "󰍷 删除用户", ID: "delete"},
		{Key: '0', Label: "󰌍 返回", ID: "back"},
	})
	return v
}

func (v *UserView) Name() string { return "user" }

func (v *UserView) Init() tea.Cmd {
	v.step = userMenu
	return nil
}

func (v *UserView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case components.MenuSelectMsg:
		switch msg.ID {
		case "back":
			return v, tui.BackCmd
		case "list":
			v.step = userList
			return v, v.listUsers
		case "add":
			v.step = userAdd
			return v, func() tea.Msg {
				return tui.ShowOverlayMsg{
					Overlay: components.NewTextInput("用户名:", "user2"),
				}
			}
		case "rename":
			names := derived.UserNames(v.model.Store())
			if len(names) == 0 {
				v.step = userResult
				return v, func() tea.Msg {
					return tui.ShowOverlayMsg{
						Overlay: components.NewResult("暂无用户"),
					}
				}
			}
			v.step = userRenameOld
			return v, func() tea.Msg {
				return tui.ShowOverlayMsg{
					Overlay: components.NewSelectList("选择要重置的用户:", names),
				}
			}
		case "delete":
			names := derived.UserNames(v.model.Store())
			if len(names) == 0 {
				v.step = userResult
				return v, func() tea.Msg {
					return tui.ShowOverlayMsg{
						Overlay: components.NewResult("暂无用户"),
					}
				}
			}
			v.step = userDelete
			return v, func() tea.Msg {
				return tui.ShowOverlayMsg{
					Overlay: components.NewSelectList("选择要删除的用户:", names),
				}
			}
		}

	case tui.InputResultMsg:
		if msg.Cancelled {
			v.step = userMenu
			return v, nil
		}
		switch v.step {
		case userAdd:
			val := msg.Value
			return v, func() tea.Msg { return v.doAdd(val) }
		case userRenameOld:
			v.oldName = msg.Value
			v.step = userRenameNew
			return v, func() tea.Msg {
				return tui.ShowOverlayMsg{
					Overlay: components.NewTextInput("新用户名:", ""),
				}
			}
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
		return v, func() tea.Msg {
			return tui.ShowOverlayMsg{
				Overlay: components.NewResult(msg.result),
			}
		}

	case tui.ResultDismissedMsg:
		v.step = userMenu
		return v, nil

	default:
		if v.step == userMenu {
			var cmd tea.Cmd
			v.menu, cmd = v.menu.Update(msg)
			return v, cmd
		}
	}
	return v, nil
}

func (v *UserView) View() string {
	return tui.RenderSubMenuFrame(v.menu.View(), tui.DefaultSubMenuHint, tui.SeparatorWidth)
}

type userActionDoneMsg struct{ result string }

func (v *UserView) listUsers() tea.Msg {
	users := user.List(v.model.Store())
	var sb strings.Builder
	sb.WriteString("用户列表\n\n")
	if len(users) == 0 {
		sb.WriteString("暂无用户")
	}
	for _, u := range users {
		sb.WriteString(fmt.Sprintf("  %s  (%d 协议, %d 路由)\n",
			u.Name, len(u.Memberships), u.RouteCount))
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
