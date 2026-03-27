package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/derived"
	"go-proxy/internal/store"
)

// FocusPanel indicates which panel has keyboard focus.
type FocusPanel int

const (
	FocusLeft FocusPanel = iota
	FocusRight
)

// View is the interface each TUI view must implement.
type View interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (View, tea.Cmd)
	View() string
	Name() string
	HasInline() bool
}

// viewDisplayName maps view IDs to Chinese display names for breadcrumbs/titles.
var viewDisplayName = map[string]string{
	"protocol-install": "安装协议",
	"protocol-remove":  "卸载协议",
	"user":             "用户管理",
	"routing":          "分流管理",
	"service":          "协议管理",
	"subscription":     "订阅管理",
	"config":           "查看配置",
	"logs":             "运行日志",
	"core":             "内核管理",
	"network":          "网络管理",
	"self-update":      "脚本更新",
	"uninstall":        "卸载服务",
}

// Model is the root Bubble Tea model.
type Model struct {
	nav          NavState
	store        *store.Store
	version      string
	width        int
	height       int
	views        map[string]View
	current      string     // name of the active sub-view ("" = none)
	focus        FocusPanel // which panel has keyboard focus
	mainMenu     MenuModel
	dashCache    derived.DashCache
	splitPanel   bool // true when terminal >= 80 cols
	leftWidth    int  // left panel width
	rightWidth   int  // right panel width
	contentWidth int  // inner content width for sub-views
	dragging     bool // true during panel divider drag
	manualWidth  int  // manually-set left width via drag (0 = auto-calculate)
	exitMessage  string
}

// Main menu items — defined once, used by root model.
var mainMenuItems = []MenuItem{
	{Key: '1', Label: "󰒍 安装协议", ID: "protocol-install"},
	{Key: '2', Label: "󰆴 卸载协议", ID: "protocol-remove"},
	{Key: '3', Label: "󰁥 用户管理", ID: "user"},
	{Key: '4', Label: "󰛳 分流管理", ID: "routing"},
	{Key: '5', Label: "󰒓 协议管理", ID: "service"},
	{Key: '6', Label: "󰑫 订阅管理", ID: "subscription"},
	{Key: '7', Label: "󰈔 查看配置", ID: "config"},
	{Key: '8', Label: "󰌱 运行日志", ID: "logs"},
	{Key: '9', Label: "󰚗 内核管理", ID: "core"},
	{Key: 'a', Label: "󰀂 网络管理", ID: "network"},
	{Key: 'b', Label: "󰁪 脚本更新", ID: "self-update"},
	{Key: 'c', Label: "󰩺 卸载服务", ID: "uninstall"},
	{Key: '0', Label: "󰗼 完全退出", ID: "quit"},
}

// NewModel creates the root model.
func NewModel(s *store.Store, version string) Model {
	m := Model{
		store:        s,
		version:      version,
		views:        make(map[string]View),
		width:        80,
		height:       24,
		contentWidth: SeparatorWidth,
		mainMenu:     NewMenu("", mainMenuItems).SetColumns(1),
	}
	m.recalcLayout()
	return m
}

// RegisterView adds a view to the model.
func (m *Model) RegisterView(v View) {
	m.views[v.Name()] = v
}

// Store returns the store for views to access.
func (m *Model) Store() *store.Store { return m.store }

// Version returns the version string.
func (m *Model) Version() string { return m.version }

// Width returns the terminal width.
func (m *Model) Width() int { return m.width }

// Height returns the terminal height.
func (m *Model) Height() int { return m.height }

// ContentWidth returns the appropriate width for sub-view content rendering.
// In split-panel mode this is the right panel inner width; otherwise SeparatorWidth.
func (m *Model) ContentWidth() int { return m.contentWidth }

// SetExitMessage sets a message to be printed to stdout after the TUI exits.
func (m *Model) SetExitMessage(msg string) { m.exitMessage = msg }

// ExitMessage returns the post-exit message (empty if none).
func (m Model) ExitMessage() string { return m.exitMessage }

// recalcLayout updates panel widths based on terminal dimensions.
func (m *Model) recalcLayout() {
	if m.width < 80 {
		// Single-panel fallback.
		m.splitPanel = false
		m.contentWidth = SeparatorWidth
		if m.contentWidth > m.width {
			m.contentWidth = m.width
		}
		return
	}

	m.splitPanel = true

	if m.manualWidth > 0 {
		m.leftWidth = m.manualWidth
		if m.leftWidth > m.width-30 {
			m.leftWidth = m.width - 30
		}
		if m.leftWidth < 24 {
			m.leftWidth = 24
		}
	} else {
		// Left panel: 30% of width, clamped to [32, 40].
		m.leftWidth = m.width * 30 / 100
		if m.leftWidth < 32 {
			m.leftWidth = 32
		}
		if m.leftWidth > 40 {
			m.leftWidth = 40
		}
	}

	m.rightWidth = m.width - m.leftWidth

	// Content width = right panel inner (minus border + padding: 2+2=4).
	// No SeparatorWidth cap in split-panel — let content fill the panel.
	m.contentWidth = m.rightWidth - 4
	if m.contentWidth < 30 {
		m.contentWidth = 30
	}
}

// Init satisfies tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages at the root level.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalcLayout()

		// Forward resize to current sub-view with actual dimensions.
		if m.current != "" {
			if v, ok := m.views[m.current]; ok {
				resizeMsg := ViewResizeMsg{ContentWidth: m.contentWidth, ContentHeight: m.height - 2}
				newView, cmd := v.Update(resizeMsg)
				m.views[m.current] = newView
				return m, cmd
			}
		}
		return m, nil

	case tea.MouseMsg:
		if !m.splitPanel {
			return m, nil
		}

		// Check if this is a main divider drag operation.
		onMainDivider := msg.X >= m.leftWidth-3 && msg.X <= m.leftWidth+3

		switch msg.Action {
		case tea.MouseActionPress:
			if msg.Button == tea.MouseButtonLeft && onMainDivider {
				m.dragging = true
				return m, nil
			}
		case tea.MouseActionMotion:
			if m.dragging {
				newLeft := msg.X
				minLeft := 24
				maxLeft := m.width * 60 / 100
				if newLeft < minLeft {
					newLeft = minLeft
				}
				if newLeft > maxLeft {
					newLeft = maxLeft
				}
				m.manualWidth = newLeft
				m.leftWidth = newLeft
				m.rightWidth = m.width - m.leftWidth
				m.contentWidth = m.rightWidth - 4
				if m.contentWidth < 30 {
					m.contentWidth = 30
				}
				return m, nil
			}
		case tea.MouseActionRelease:
			if m.dragging {
				m.dragging = false
				return m, nil
			}
		}

		// Forward mouse events inside the right panel to the current view.
		// Right panel inner area starts at leftWidth + 2 (border + padding).
		rightInnerX := m.leftWidth + 2
		if m.current != "" && msg.X >= rightInnerX {
			relMsg := msg
			relMsg.X = msg.X - rightInnerX
			relMsg.Y = msg.Y - 1 // account for top border
			if v, ok := m.views[m.current]; ok {
				newView, cmd := v.Update(SubSplitMouseMsg{MouseMsg: relMsg})
				m.views[m.current] = newView
				return m, cmd
			}
		}
		return m, nil

	case NavigateMsg:
		if msg.Name == "main-menu" {
			m.current = ""
			m.focus = FocusLeft
			m.mainMenu = m.mainMenu.SetActiveID("")
			m.nav.Clear()
			return m, nil
		}
		m.nav.Clear()
		m.nav.Push(msg.Name)
		m.current = msg.Name
		m.focus = FocusRight
		m.mainMenu = m.mainMenu.SetActiveID(msg.Name).SetDim(true)
		if v, ok := m.views[msg.Name]; ok {
			initCmd := v.Init()
			// Send actual content dimensions since views' *Model pointer is stale.
			resizeMsg := ViewResizeMsg{ContentWidth: m.contentWidth, ContentHeight: m.height - 2}
			newView, resizeCmd := v.Update(resizeMsg)
			m.views[msg.Name] = newView
			return m, tea.Batch(initCmd, resizeCmd)
		}
		return m, nil

	case BackMsg:
		m.current = ""
		m.focus = FocusLeft
		m.mainMenu = m.mainMenu.SetActiveID("").SetDim(false)
		m.nav.Clear()
		return m, nil

	case InputResultMsg, ConfirmResultMsg, ResultDismissedMsg, OverlaySelectMsg:
		// Forward component result messages to the active view.
		if v, ok := m.views[m.current]; ok {
			newView, cmd := v.Update(msg)
			m.views[m.current] = newView
			return m, cmd
		}
		return m, nil

	case MenuSelectMsg:
		// Main menu selection (only when focus is left).
		if m.focus == FocusLeft {
			if msg.ID == "quit" {
				return m, tea.Quit
			}
			return m, func() tea.Msg {
				return NavigateMsg{Name: msg.ID}
			}
		}

	case tea.KeyMsg:
		if key.Matches(msg, Keys.Quit) && !(m.current == "subscription" && m.focus == FocusRight) {
			return m, tea.Quit
		}

		// Left/Right arrows toggle focus between panels.
		if m.splitPanel && m.current != "" {
			if key.Matches(msg, Keys.Left) && m.focus == FocusRight {
				// If view has SubSplit with right focus, forward Left to
				// toggle SubSplit focus (Level 3 → Level 2) instead of
				// jumping to the main left panel (Level 1).
				if v, ok := m.views[m.current]; ok {
					if ssf, ok := v.(SubSplitFocuser); ok && ssf.IsSubSplitRightFocused() {
						newView, cmd := v.Update(msg)
						m.views[m.current] = newView
						return m, cmd
					}
				}
				m.focus = FocusLeft
				m.mainMenu = m.mainMenu.SetDim(false)
				return m, nil
			}
			if key.Matches(msg, Keys.Right) && m.focus == FocusLeft {
				m.focus = FocusRight
				m.mainMenu = m.mainMenu.SetDim(true)
				return m, nil
			}
		}

		// Global keys.
		switch {
		case key.Matches(msg, Keys.QuitQ):
			if m.focus == FocusLeft && m.current == "" {
				return m, tea.Quit
			}
		case key.Matches(msg, Keys.Back):
			if m.focus == FocusLeft && m.current == "" {
				return m, tea.Quit
			}
			if m.focus == FocusRight {
				// Forward Esc to view for internal back navigation.
				// Views send BackCmd when at their top-level step.
				if v, ok := m.views[m.current]; ok {
					newView, cmd := v.Update(msg)
					m.views[m.current] = newView
					return m, cmd
				}
				return m, BackCmd
			}
		}
	}

	// Route input to the focused panel.
	if m.focus == FocusLeft {
		var cmd tea.Cmd
		m.mainMenu, cmd = m.mainMenu.Update(msg)
		return m, cmd
	}

	if v, ok := m.views[m.current]; ok {
		newView, cmd := v.Update(msg)
		m.views[m.current] = newView
		return m, cmd
	}

	return m, nil
}

// View renders the current state.
func (m Model) View() string {
	if m.splitPanel {
		return m.viewSplitPanel()
	}
	return m.viewSinglePanel()
}

// viewSplitPanel renders the two-panel layout.
func (m Model) viewSplitPanel() string {
	leftContent := m.renderLeftPanel()
	rightContent := m.renderRightPanel()

	// Apply border styles based on focus.
	var leftStyle, rightStyle lipgloss.Style
	if m.focus == FocusLeft {
		leftStyle = LeftPanelFocusedStyle
		rightStyle = RightPanelStyle
	} else {
		leftStyle = LeftPanelStyle
		rightStyle = RightPanelFocusedStyle
	}

	// Yellow border highlight during drag resize.
	if m.dragging {
		leftStyle = leftStyle.BorderForeground(ColorDragBorder)
		rightStyle = rightStyle.BorderForeground(ColorDragBorder)
	}

	leftPanel := leftStyle.Width(m.leftWidth - 2).Height(m.height - 2).Render(leftContent)
	rightPanel := rightStyle.Width(m.rightWidth - 2).Height(m.height - 2).Render(rightContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
}

// viewSinglePanel renders the fallback full-screen layout for narrow terminals.
func (m Model) viewSinglePanel() string {
	var content string

	if m.current == "" {
		// Show full-screen main menu with dashboard.
		w := m.width
		dashboard := RenderDashboard(m.dashCache.Get(m.store), m.version, w)
		sepWidth := m.contentWidth
		sep := SeparatorDouble(sepWidth)
		hint := RenderFooterHint("退出(esc) | 选择(↑↓) | 确认(enter)", sepWidth)
		body := lipgloss.JoinVertical(lipgloss.Center,
			dashboard,
			m.mainMenu.View(),
			sep,
			hint,
			"",
		)
		framed := OuterFrameStyle.Width(sepWidth).Render(body)
		content = lipgloss.NewStyle().Width(w).Align(lipgloss.Center).Render(framed)
	} else if v, ok := m.views[m.current]; ok {
		// Views return body-only; add hint footer for single-panel mode.
		viewContent := v.View()
		w := m.contentWidth
		hintSep := SeparatorDouble(w)
		hintLine := RenderFooterHint(DefaultSubMenuHint, w)
		content = lipgloss.JoinVertical(lipgloss.Center, viewContent, hintSep, hintLine, hintSep)
	}

	return content
}

// renderLeftPanel renders the persistent left panel content.
func (m Model) renderLeftPanel() string {
	innerW := m.leftWidth - 4
	innerH := m.height - 2 // panel inner height (border = 2 rows)
	if innerH < 0 {
		innerH = 0
	}

	dashboard := RenderCompactDashboard(m.dashCache.Get(m.store), m.version, innerW)

	menuView := m.mainMenu.SetWidth(innerW).View()

	var hint string
	if m.current == "" {
		hint = RenderFooterHint("退出(esc) | 选择(↑↓)", innerW)
	} else {
		hint = RenderFooterHint("切换(←→) | 选择(↑↓)", innerW)
	}

	// Use Height to fill top section, pushing hint flush with bottom border.
	topContent := lipgloss.JoinVertical(lipgloss.Left, dashboard, menuView)
	hintH := lipgloss.Height(hint)
	topSection := lipgloss.NewStyle().Height(max(innerH-hintH, 0)).Render(topContent)

	return lipgloss.JoinVertical(lipgloss.Left, topSection, hint)
}

// renderRightPanel renders the dynamic right panel content.
func (m Model) renderRightPanel() string {
	innerW := m.rightWidth - 4
	innerH := m.height - 2 // panel inner height (border = 2 rows)
	if innerH < 0 {
		innerH = 0
	}

	if m.current == "" {
		return m.renderWelcome(innerW)
	}

	// Title bar.
	displayName := viewDisplayName[m.current]
	if displayName == "" {
		displayName = m.current
	}
	titleBar := SubMenuTitleStyle.Width(innerW).Render("@ " + displayName)

	// Breadcrumb.
	breadcrumb := m.renderBreadcrumb(innerW)

	// Separator aligned with left panel.
	sep1 := SeparatorDouble(innerW)

	// Sub-view content.
	var viewContent string
	if v, ok := m.views[m.current]; ok {
		viewContent = v.View()
	}

	// Use Height to fill the full inner area so SubSplit dividers reach the bottom border.
	topContent := lipgloss.JoinVertical(lipgloss.Left, titleBar, breadcrumb, sep1, viewContent)
	topSection := lipgloss.NewStyle().Width(innerW).Height(innerH).Render(topContent)

	return topSection
}

// renderBreadcrumb renders the navigation breadcrumb trail.
func (m Model) renderBreadcrumb(width int) string {
	crumbs := m.nav.Breadcrumb()
	if len(crumbs) == 0 {
		return ""
	}

	var parts []string
	parts = append(parts, BreadcrumbDimStyle.Render("主菜单"))
	for i, name := range crumbs {
		displayName := viewDisplayName[name]
		if displayName == "" {
			displayName = name
		}
		sep := BreadcrumbDimStyle.Render(" › ")
		if i == len(crumbs)-1 {
			parts = append(parts, sep+BreadcrumbActiveStyle.Render(displayName))
		} else {
			parts = append(parts, sep+BreadcrumbDimStyle.Render(displayName))
		}
	}

	line := strings.Join(parts, "")
	return lipgloss.NewStyle().Width(width).Render(line)
}

// renderWelcome renders the welcome/home screen for the right panel.
func (m Model) renderWelcome(width int) string {
	title := lipgloss.NewStyle().
		Foreground(ColorTitle).
		Bold(true).
		Width(width).
		Align(lipgloss.Center).
		Render("go-proxy 快捷指令：gproxy")

	sub := lipgloss.NewStyle().
		Foreground(ColorBlack).
		Width(width).
		Align(lipgloss.Center).
		Render("仓库地址: https://github.com/dhwang2/go-proxy")

	sep := SeparatorDouble(width)

	tips := lipgloss.NewStyle().Foreground(ColorLabel).Render(strings.Join([]string{
		"  开始:",
		"  1. 选择左侧菜单项打开功能面板",
		"  2. ← → 键切换左右面板焦点",
		"  3. 快捷键: 1-9, a-c 直接选择菜单项",
	}, "\n"))

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		sub,
		sep,
		tips,
	)
}
