package pages

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"go-proxy/internal/tui"
	"go-proxy/internal/tui/dialog"
	"go-proxy/internal/user"
)

// UserPage handles user management.
type UserPage struct {
	state *tui.AppState
	list  *tview.List
}

// NewUserPage creates the user management page.
func NewUserPage(state *tui.AppState) *UserPage {
	p := &UserPage{state: state}

	p.list = tview.NewList().
		ShowSecondaryText(false).
		SetHighlightFullLine(true).
		SetMainTextColor(tcell.ColorWhite).
		SetSelectedTextColor(tcell.ColorTeal).
		SetSelectedBackgroundColor(tcell.ColorDarkSlateGray)
	p.list.SetTitle(" User Management ").
		SetTitleColor(tcell.ColorTeal).
		SetBorder(true).
		SetBorderColor(tcell.ColorTeal)

	return p
}

func (p *UserPage) Name() string          { return "user" }
func (p *UserPage) Root() tview.Primitive { return p.list }

func (p *UserPage) OnEnter() {
	p.list.Clear()
	p.list.AddItem("用户列表  List Users", "", '1', p.listUsers)
	p.list.AddItem("添加用户  Add User", "", '2', p.addUser)
	p.list.AddItem("重命名用户  Rename User", "", '3', p.renameUser)
	p.list.AddItem("删除用户  Delete User", "", '4', p.deleteUser)
	p.list.AddItem("返回  Back", "", '0', func() {
		p.state.Back()
	})
	p.state.App.SetFocus(p.list)
}

func (p *UserPage) listUsers() {
	users := user.List(p.state.Store)
	var sb strings.Builder
	sb.WriteString("Users\n\n")
	if len(users) == 0 {
		sb.WriteString("No users found")
	}
	for _, u := range users {
		sb.WriteString(fmt.Sprintf("  %s  (%d protocols, %d routes)\n",
			u.Name, len(u.Memberships), u.RouteCount))
	}
	p.showResult(sb.String())
}

func (p *UserPage) addUser() {
	d := dialog.NewInput("Username:", "user2", func(val string) {
		p.state.DismissDialog("user-add")
		if val == "" {
			return
		}
		if err := user.Add(p.state.Store, val); err != nil {
			p.showResult("Error: " + err.Error())
			return
		}
		if err := p.state.Store.Apply(); err != nil {
			p.showResult("Failed to save: " + err.Error())
			return
		}
		p.showResult("Added user: " + val)
	})
	p.state.ShowDialog("user-add", d)
}

func (p *UserPage) renameUser() {
	d := dialog.NewInput("Current username:", "", func(oldName string) {
		p.state.DismissDialog("user-rename-old")
		if oldName == "" {
			return
		}
		d2 := dialog.NewInput("New username:", "", func(newName string) {
			p.state.DismissDialog("user-rename-new")
			if newName == "" {
				return
			}
			if err := user.Rename(p.state.Store, oldName, newName); err != nil {
				p.showResult("Error: " + err.Error())
				return
			}
			if err := p.state.Store.Apply(); err != nil {
				p.showResult("Failed to save: " + err.Error())
				return
			}
			p.showResult(fmt.Sprintf("Renamed %s → %s", oldName, newName))
		})
		p.state.ShowDialog("user-rename-new", d2)
	})
	p.state.ShowDialog("user-rename-old", d)
}

func (p *UserPage) deleteUser() {
	d := dialog.NewInput("Username to delete:", "", func(val string) {
		p.state.DismissDialog("user-delete")
		if val == "" {
			return
		}
		if err := user.Delete(p.state.Store, val); err != nil {
			p.showResult("Error: " + err.Error())
			return
		}
		if err := p.state.Store.Apply(); err != nil {
			p.showResult("Failed to save: " + err.Error())
			return
		}
		p.showResult("Deleted user: " + val)
	})
	p.state.ShowDialog("user-delete", d)
}

func (p *UserPage) showResult(msg string) {
	d := dialog.NewResult(msg, func() {
		p.state.DismissDialog("user-result")
		p.OnEnter()
	})
	p.state.ShowDialog("user-result", d)
}
