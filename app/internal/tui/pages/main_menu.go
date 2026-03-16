package pages

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"go-proxy/internal/tui"
)

// MainMenuPage is the root menu page.
type MainMenuPage struct {
	state     *tui.AppState
	flex      *tview.Flex
	dashboard *tview.TextView
	list      *tview.List
	subPages  map[string]tui.Page
}

// NewMainMenuPage creates the main menu with dashboard header.
func NewMainMenuPage(state *tui.AppState) *MainMenuPage {
	p := &MainMenuPage{
		state:    state,
		subPages: make(map[string]tui.Page),
	}

	p.dashboard = tui.NewDashboard(state.Store, state.Version)

	p.list = tview.NewList().
		ShowSecondaryText(false).
		SetHighlightFullLine(true).
		SetMainTextColor(tcell.ColorWhite).
		SetSelectedTextColor(tcell.ColorTeal).
		SetSelectedBackgroundColor(tcell.ColorDarkSlateGray)

	p.list.SetBorder(false)

	// Menu items: key, label, page name.
	items := []struct {
		key  rune
		text string
		page string
	}{
		{'1', "安装协议  Install Protocol", "protocol-install"},
		{'2', "卸载协议  Remove Protocol", "protocol-remove"},
		{'3', "用户管理  User Management", "user"},
		{'4', "分流管理  Routing Management", "routing"},
		{'5', "协议管理  Service Management", "service"},
		{'6', "订阅管理  Subscription", "subscription"},
		{'7', "查看配置  View Configuration", "config"},
		{'8', "运行日志  Runtime Logs", "logs"},
		{'9', "内核管理  Core Management", "core"},
		{'a', "网络管理  Network Management", "network"},
		{'b', "脚本更新  Self Update", "self-update"},
		{'c', "卸载服务  Uninstall", "uninstall"},
		{'0', "退出  Exit", ""},
	}

	for _, item := range items {
		pageName := item.page
		p.list.AddItem(item.text, "", item.key, func() {
			if pageName == "" {
				state.App.Stop()
				return
			}
			if sp, ok := p.subPages[pageName]; ok {
				state.NavigateWithCallback(sp)
			} else {
				state.Navigate(pageName)
			}
		})
	}

	// Status bar.
	status := tview.NewTextView().
		SetDynamicColors(true).
		SetText("[gray]ESC: back  q: quit")
	status.SetTextAlign(tview.AlignLeft)

	p.flex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.dashboard, 7, 0, false).
		AddItem(p.list, 0, 1, true).
		AddItem(status, 1, 0, false)

	return p
}

// RegisterSubPage associates a page with a menu item for OnEnter callbacks.
func (p *MainMenuPage) RegisterSubPage(page tui.Page) {
	p.subPages[page.Name()] = page
}

func (p *MainMenuPage) Name() string          { return "main-menu" }
func (p *MainMenuPage) Root() tview.Primitive { return p.flex }
func (p *MainMenuPage) OnEnter() {
	tui.UpdateDashboard(p.dashboard, p.state.Store, p.state.Version)
	p.state.App.SetFocus(p.list)
}
