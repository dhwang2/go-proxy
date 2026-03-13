package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dhwang2/go-proxy/internal/config"
	"github.com/dhwang2/go-proxy/internal/service"
	"github.com/dhwang2/go-proxy/internal/store"
	"github.com/dhwang2/go-proxy/internal/tui/components"
	"github.com/dhwang2/go-proxy/internal/tui/layout"
)

type statusLoadedMsg struct {
	rows []service.ServiceStatus
	err  error
}

type serviceActionDoneMsg struct {
	action  string
	results []service.OperationResult
}

type protocolInstallDoneMsg struct {
	proto string
	body  string
	err   error
}

type clearToastMsg struct {
	until time.Time
}

type appModel struct {
	store    *store.Store
	services *service.Manager
	version  string

	state               MenuState
	mainMenu            components.MenuList
	protocolInstallMenu components.MenuList
	userMenu            components.MenuList
	routingMenu         components.MenuList
	routingRulesMenu    components.MenuList
	routingChainMenu    components.MenuList
	serviceMenu         components.MenuList
	configMenu          components.MenuList
	logsMenu            components.MenuList
	coreMenu            components.MenuList
	networkMenu         components.MenuList
	bbrMenu             components.MenuList
	firewallMenu        components.MenuList
	selectionMenu       components.MenuList
	statusRows          []service.ServiceStatus
	spinner             components.Spinner
	loading             bool
	toast               string
	toastIsError        bool
	toastUntil          time.Time

	textTitle      string
	textBody       string
	textHint       string
	selectionTitle string
	selectionMode  string
	inputTitle     string
	inputHint      string
	input          components.TextInput
	inputMode      string
	confirmTitle   string
	confirm        components.Confirm
	confirmMode    string
	returnState    MenuState
	actionContext  map[string]string
	width          int
	height         int
}

func Run(st *store.Store, svc *service.Manager, version string) error {
	if !isInteractiveTerminal() {
		return fmt.Errorf("interactive TUI requires a TTY")
	}
	model := newAppModel(st, svc, version)
	program := tea.NewProgram(model, tea.WithAltScreen())
	_, err := program.Run()
	return err
}

func newAppModel(st *store.Store, svc *service.Manager, version string) appModel {
	cfg, _ := config.Pretty(st.Config)
	return appModel{
		store:               st,
		services:            svc,
		version:             version,
		state:               StateMainMenu,
		mainMenu:            newMenuList(MainMenuItems),
		protocolInstallMenu: newMenuList(ProtocolInstallMenuItems),
		userMenu:            newMenuList(UserMenuItems),
		routingMenu:         newMenuList(RoutingMenuItems),
		routingRulesMenu:    newMenuList(RoutingRulesMenuItems),
		routingChainMenu:    newMenuList(RoutingChainMenuItems),
		serviceMenu:         newMenuList(ServiceMenuItems),
		configMenu:          newMenuList(ConfigMenuItems),
		logsMenu:            newMenuList(LogsMenuItems),
		coreMenu:            newMenuList(CoreMenuItems),
		networkMenu:         newMenuList(NetworkMenuItems),
		bbrMenu:             newMenuList(BBRMenuItems),
		firewallMenu:        newMenuList(FirewallMenuItems),
		spinner:             components.NewSpinner("加载服务状态"),
		loading:             true,
		textBody:            cfg,
		actionContext:       map[string]string{},
	}
}

func (m appModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Init(), m.loadStatusCmd("加载服务状态..."))
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.loading {
		m.spinner, cmd = m.spinner.Update(msg)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, cmd
	case clearToastMsg:
		if !m.toastUntil.IsZero() && m.toastUntil.Equal(msg.until) {
			m.toast = ""
			m.toastIsError = false
			m.toastUntil = time.Time{}
		}
		return m, cmd
	case statusLoadedMsg:
		m.loading = false
		if msg.err != nil {
			toastCmd := (&m).setToast("状态刷新失败: "+msg.err.Error(), true)
			return m, batchCmd(cmd, toastCmd)
		}
		m.statusRows = msg.rows
		toastCmd := (&m).setToast("服务状态已刷新", false)
		return m, batchCmd(cmd, toastCmd)
	case serviceActionDoneMsg:
		m.loading = false
		toast, isErr := summarizeServiceAction(msg.action, msg.results)
		toastCmd := (&m).setToast(toast, isErr)
		return m, tea.Batch(toastCmd, m.loadStatusCmd("刷新服务状态..."))
	case protocolInstallDoneMsg:
		m.loading = false
		if msg.err != nil {
			toastCmd := (&m).setToast(msg.err.Error(), true)
			return m, batchCmd(cmd, toastCmd)
		}
		m = m.openTextView("安装协议", msg.body, StateProtocolInstallMenu)
		toastCmd := (&m).setToast(msg.proto+" 已安装", false)
		return m, tea.Batch(toastCmd, m.loadStatusCmd("刷新服务状态..."))
	case tea.KeyMsg:
		return m.handleKey(msg, cmd)
	}

	return m, cmd
}

func (m appModel) handleKey(msg tea.KeyMsg, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	key := normalizeKey(msg)
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	}

	switch m.state {
	case StateTextView, StateConfigView, StateLogsView:
		return m.updateTextState(key, spinCmd)
	case StateInputPrompt:
		return m.updateInputState(msg, spinCmd)
	case StateConfirmPrompt:
		return m.updateConfirmState(key, spinCmd)
	default:
		if m.isMenuState() {
			return m.updateMenuState(key, spinCmd)
		}
		return m, spinCmd
	}
}

func (m appModel) updateMainMenu(key string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	m.state = StateMainMenu
	return m.updateMenuState(key, spinCmd)
}

func (m appModel) selectMainMenu(key string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	m.state = StateMainMenu
	return m.selectMenu(key, spinCmd)
}

func (m appModel) updateServiceMenu(key string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	m.state = StateServiceManagement
	return m.updateMenuState(key, spinCmd)
}

func (m appModel) selectServiceMenu(key string, spinCmd tea.Cmd) (tea.Model, tea.Cmd) {
	m.state = StateServiceManagement
	return m.selectMenu(key, spinCmd)
}

func (m appModel) View() string {
	if m.width == 0 {
		m.width = 108
	}
	w := minInt(76, m.width-4)

	// Header card (rounded border, cyan accent)
	headerContent := fmt.Sprintf("%s  %s              %s\n%s",
		brandStyle.Render("多协议代理"),
		mutedStyle.Render("一键部署"),
		versionStyle.Render("[服务端]"),
		mutedStyle.Render("作者：dhwang            快捷命令：proxy"))
	headerBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#5fd7ff")).
		Padding(0, 2).
		Width(w).
		Render(headerContent)

	// Dashboard panel (normal border, subtle gray)
	dash := layout.Dashboard(m.store, m.statusRows, w)

	// Body content
	var body string
	switch m.state {
	case StateTextView, StateConfigView, StateLogsView:
		body = m.viewTextBody()
	case StateInputPrompt:
		body = m.viewInputBody()
	case StateConfirmPrompt:
		body = m.viewConfirmBody()
	default:
		body = m.viewMenuBody()
	}

	// Section header for non-main-menu states
	if m.state != StateMainMenu {
		body = layout.LabeledDivider(m.sectionTitle(), w) + "\n" + body
	}

	if m.loading {
		body += "\n\n" + warnStyle.Render(m.spinner.View())
	}
	if m.toast != "" {
		prefix := "  ✓ "
		style := okStyle
		if m.toastIsError {
			prefix = "  ✗ "
			style = errStyle
		}
		body += "\n\n" + style.Render(prefix+m.toast)
	}

	parts := []string{"", headerBox, dash, body, ""}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m appModel) sectionTitle() string {
	switch m.state {
	case StateSelectionMenu:
		if m.selectionTitle != "" {
			return m.selectionTitle
		}
	case StateTextView:
		if m.textTitle != "" {
			return m.textTitle
		}
	case StateInputPrompt:
		if m.inputTitle != "" {
			return m.inputTitle
		}
	case StateConfirmPrompt:
		if m.confirmTitle != "" {
			return m.confirmTitle
		}
	}
	return stateTitle(m.state)
}

func (m appModel) loadStatusCmd(message string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		rows, err := m.services.AllStatuses(ctx)
		return statusLoadedMsg{rows: rows, err: err}
	}
}

func (m appModel) serviceActionCmd(action string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		results := m.services.OperateAll(ctx, action)
		return serviceActionDoneMsg{action: action, results: results}
	}
}

func (m *appModel) setToast(message string, isErr bool) tea.Cmd {
	message = strings.TrimSpace(message)
	m.toast = message
	m.toastIsError = isErr
	if message == "" {
		m.toastUntil = time.Time{}
		return nil
	}
	m.toastUntil = time.Now().Add(4 * time.Second)
	until := m.toastUntil
	wait := time.Until(until)
	if wait <= 0 {
		wait = 50 * time.Millisecond
	}
	return tea.Tick(wait, func(time.Time) tea.Msg {
		return clearToastMsg{until: until}
	})
}

func batchCmd(cmds ...tea.Cmd) tea.Cmd {
	nonNil := make([]tea.Cmd, 0, len(cmds))
	for _, cmd := range cmds {
		if cmd != nil {
			nonNil = append(nonNil, cmd)
		}
	}
	if len(nonNil) == 0 {
		return nil
	}
	if len(nonNil) == 1 {
		return nonNil[0]
	}
	return tea.Batch(nonNil...)
}

func summarizeServiceAction(action string, results []service.OperationResult) (string, bool) {
	ok := 0
	failed := 0
	skipped := 0
	for _, r := range results {
		switch r.Status {
		case "ok":
			ok++
		case "skipped":
			skipped++
		default:
			failed++
		}
	}
	msg := fmt.Sprintf("%s completed: ok=%d skipped=%d failed=%d", action, ok, skipped, failed)
	return msg, failed > 0
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func isInteractiveTerminal() bool {
	stdinInfo, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	stdoutInfo, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (stdinInfo.Mode()&os.ModeCharDevice) != 0 && (stdoutInfo.Mode()&os.ModeCharDevice) != 0
}

func normalizeKey(msg tea.KeyMsg) string {
	if msg.Type == tea.KeyEnter {
		return "enter"
	}
	key := strings.ToLower(msg.String())
	switch key {
	case "enter", "ctrl+m", "ctrl+j", "\r", "\n", "\r\n":
		return "enter"
	default:
		return strings.TrimSpace(key)
	}
}
