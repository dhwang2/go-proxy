package views

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/cert"
	"go-proxy/internal/protocol"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type ProtocolInstallView struct {
	tui.InlineState
	model         *tui.Model
	menu          tui.MenuModel
	split         tui.SubSplitModel
	step          protoInstallStep
	pendingType   protocol.Type
	pendingPort   int
	pendingDomain string
	pendingEmail  string
	lastResult    *protocol.InstallResult
}

type protoInstallStep int

const (
	protoInstallMenu protoInstallStep = iota
	protoInstallPort
	protoInstallDomain
	protoInstallEmail
	protoInstallCert
	protoInstallResult
	protoInstallShadowTLSPrompt
)

func NewProtocolInstallView(model *tui.Model) *ProtocolInstallView {
	return &ProtocolInstallView{model: model}
}

func (v *ProtocolInstallView) Name() string { return "protocol-install" }

func (v *ProtocolInstallView) setFocus(left bool) {
	v.split.SetFocusLeft(left)
	v.menu = v.menu.SetDim(!left)
}

func (v *ProtocolInstallView) Init() tea.Cmd {
	v.step = protoInstallMenu
	v.split.SetFocusLeft(true)
	v.split.SetSize(v.model.ContentWidth(), v.model.Height()-5)
	types := protocol.InstallableTypes()
	specs := protocol.Specs()
	items := make([]tui.MenuItem, 0, len(types)+1)
	for i, t := range types {
		k := rune('1' + i)
		items = append(items, tui.MenuItem{
			Key:   k,
			Label: specs[t].DisplayName,
			ID:    string(t),
		})
	}
	v.menu = v.menu.SetItems(items)
	return nil
}

func (v *ProtocolInstallView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
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
	if v.split.Enabled() && v.step != protoInstallMenu && v.split.FocusLeft() {
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
		// Do not auto-preview — triggerMenuAction starts the install flow.
		return v, nil
	case tui.MenuSelectMsg:
		v.setFocus(false)
		return v, v.triggerMenuAction(msg.ID)

	case tui.InputResultMsg:
		if msg.Cancelled {
			v.step = protoInstallMenu
			v.setFocus(true)
			return v, nil
		}
		switch v.step {
		case protoInstallPort:
			return v.handlePortInput(msg.Value)
		case protoInstallDomain:
			return v.handleDomainInput(msg.Value)
		case protoInstallEmail:
			return v.handleEmailInput(msg.Value)
		}

	case certDoneMsg:
		if msg.err != nil {
			v.step = protoInstallResult
			return v, v.SetInline(components.NewResult("证书申请失败: " + msg.err.Error()))
		}
		// Cert done — proceed to install with cached port.
		v.step = protoInstallResult
		port := v.pendingPort
		pt := v.pendingType
		return v, tea.Batch(
			v.SetInline(components.NewSpinner("正在安装依赖...")),
			func() tea.Msg {
				return v.doInstallWithPort(pt, port)
			},
		)

	case protoInstallDoneMsg:
		v.step = protoInstallResult
		v.lastResult = msg.installResult
		// If snell was just installed, prompt for shadow-tls.
		if msg.installResult != nil && v.pendingType == protocol.Snell {
			v.step = protoInstallShadowTLSPrompt
			resultText := msg.result + "\n\n是否配置 shadow-tls 保护此端口?"
			return v, v.SetInline(components.NewConfirm(resultText))
		}
		return v, v.SetInline(components.NewResult(msg.result))

	case tui.ConfirmResultMsg:
		if v.step == protoInstallShadowTLSPrompt {
			if msg.Confirmed && v.lastResult != nil {
				snellPort := v.lastResult.Port
				return v, tea.Batch(
					v.SetInline(components.NewSpinner("正在配置 shadow-tls...")),
					func() tea.Msg {
						return v.doShadowTLSForSnell(snellPort)
					},
				)
			}
			// User declined; show the original result.
			v.step = protoInstallResult
			return v, v.SetInline(components.NewResult(fmt.Sprintf(
				"✓ 依赖安装完成\n✓ 协议配置写入成功\n✓ 服务已重启\n\n协议: snell  端口: %d\nCredential: %s",
				v.lastResult.Port, v.lastResult.Credential)))
		}

	case tui.ResultDismissedMsg:
		cmd := v.Init()
		return v, cmd

	default:
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyEsc {
			return v, tui.BackCmd
		}
		// Left/Right arrow toggles sub-split focus.
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if v.split.Enabled() && v.step != protoInstallMenu {
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
		if v.step == protoInstallMenu {
			var cmd tea.Cmd
			v.menu, cmd = v.menu.Update(msg)
			return v, cmd
		}
	}
	return v, inlineCmd
}

func (v *ProtocolInstallView) IsSubSplitRightFocused() bool {
	return v.split.Enabled() && !v.split.FocusLeft()
}

func (v *ProtocolInstallView) View() string {
	if v.step == protoInstallMenu || !v.split.Enabled() {
		if v.HasInline() {
			return v.ViewInline()
		}
		return tui.RenderSubMenuBody(v.menu.View(), v.model.ContentWidth())
	}

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

// triggerMenuAction executes the action for the given menu item ID.
// All protocols go to port input first (shell-proxy order).
func (v *ProtocolInstallView) triggerMenuAction(id string) tea.Cmd {
	v.pendingType = protocol.Type(id)
	v.pendingPort = 0
	v.pendingDomain = ""
	v.pendingEmail = ""
	defaultPort := v.computeDefaultPort(v.pendingType)
	v.step = protoInstallPort
	return v.SetInline(components.NewTextInput("端口号:", fmt.Sprintf("%d", defaultPort)))
}

// handlePortInput processes the submitted port and decides next step.
func (v *ProtocolInstallView) handlePortInput(portStr string) (tui.View, tea.Cmd) {
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	if port <= 0 || port > 65535 {
		defaultPort := v.computeDefaultPort(v.pendingType)
		v.step = protoInstallPort
		return v, v.SetInline(components.NewTextInput("端口号无效，请重新输入:", fmt.Sprintf("%d", defaultPort)))
	}
	v.pendingPort = port

	spec := protocol.Specs()[v.pendingType]
	if spec.NeedsTLS && !spec.UsesReality {
		// TLS non-Reality: ask for domain next.
		v.step = protoInstallDomain
		existing := cert.ReadDomain()
		return v, v.SetInline(components.NewTextInput("域名 (用于 TLS 证书):", existing))
	}
	// Non-TLS or Reality: go straight to install.
	pt := v.pendingType
	return v, tea.Batch(
		v.SetInline(components.NewSpinner("正在安装依赖...")),
		func() tea.Msg {
			return v.doInstallWithPort(pt, port)
		},
	)
}

// handleDomainInput processes the submitted domain and decides next step.
func (v *ProtocolInstallView) handleDomainInput(domain string) (tui.View, tea.Cmd) {
	if !cert.IsValidDomain(domain) {
		v.step = protoInstallDomain
		return v, v.SetInline(components.NewTextInput("域名无效，请重新输入:", domain))
	}
	v.pendingDomain = domain
	if cert.CertExists(domain) {
		// Cert already exists — go straight to install.
		pt := v.pendingType
		port := v.pendingPort
		return v, tea.Batch(
			v.SetInline(components.NewSpinner("正在安装依赖...")),
			func() tea.Msg {
				return v.doInstallWithPort(pt, port)
			},
		)
	}
	// Need to issue cert — ask for email first.
	v.step = protoInstallEmail
	defaultEmail := "admin@" + domain
	return v, v.SetInline(components.NewTextInput("邮箱 (用于 Let's Encrypt):", defaultEmail))
}

// handleEmailInput processes the submitted email and starts cert issuance.
func (v *ProtocolInstallView) handleEmailInput(email string) (tui.View, tea.Cmd) {
	v.pendingEmail = email
	v.step = protoInstallCert
	d := v.pendingDomain
	e := email
	return v, tea.Batch(
		v.SetInline(components.NewSpinner("证书申请中...")),
		func() tea.Msg {
			err := cert.EnsureCertificate(context.Background(), d, e, nil)
			return certDoneMsg{err: err}
		},
	)
}

type certDoneMsg struct {
	err error
}

type protoInstallDoneMsg struct {
	result        string
	installResult *protocol.InstallResult
}

func (v *ProtocolInstallView) collectUsedPorts() map[int]bool {
	var ports []int
	for _, ib := range v.model.Store().SingBox.Inbounds {
		ports = append(ports, ib.ListenPort)
	}
	if v.model.Store().SnellConf != nil {
		if p := v.model.Store().SnellConf.Port(); p > 0 {
			ports = append(ports, p)
		}
	}
	return protocol.CollectUsedPorts(ports)
}

func (v *ProtocolInstallView) computeDefaultPort(pt protocol.Type) int {
	return protocol.DefaultPort(pt, v.collectUsedPorts())
}

func (v *ProtocolInstallView) doInstallWithPort(pt protocol.Type, port int) tea.Msg {
	params := protocol.InstallParams{
		ProtoType: pt,
		Port:      port,
		UserName:  "user",
		Domain:    v.pendingDomain,
	}

	// Provision dependencies (download binaries, create systemd services).
	ctx := context.Background()
	depSteps := protocol.ProvisionDeps(ctx, pt, params)
	depReport := protocol.FormatDepSteps(depSteps)

	if protocol.HasDepError(depSteps) {
		msg := "依赖安装失败\n\n" + depReport
		return protoInstallDoneMsg{result: msg}
	}

	// Install protocol configuration.
	result, err := protocol.Install(v.model.Store(), params)
	if err != nil {
		msg := "协议安装失败: " + err.Error()
		if depReport != "" {
			msg = "✓ 依赖安装完成\n\n" + depReport + "\n" + msg
		}
		return protoInstallDoneMsg{result: msg}
	}
	if err := v.model.Store().Apply(); err != nil {
		return protoInstallDoneMsg{result: "保存失败: " + err.Error()}
	}

	var lines []string
	if v.pendingDomain != "" {
		lines = append(lines, "✓ 证书申请成功")
	}
	if depReport != "" {
		lines = append(lines, "✓ 依赖安装完成")
	}
	lines = append(lines, "✓ 协议配置写入成功")
	lines = append(lines, "✓ 服务已重启")
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("协议: %s  端口: %d", pt, result.Port))
	if result.Credential != "" {
		lines = append(lines, "Credential: "+result.Credential)
	}
	if result.PublicKey != "" {
		lines = append(lines, "Public Key: "+result.PublicKey)
	}
	if depReport != "" {
		lines = append(lines, "")
		lines = append(lines, depReport)
	}
	msg := strings.Join(lines, "\n")
	return protoInstallDoneMsg{result: msg, installResult: result}
}

func (v *ProtocolInstallView) doShadowTLSForSnell(snellPort int) tea.Msg {
	// ShadowTLS listens on its own port, routes to snell backend.
	used := v.collectUsedPorts()
	used[snellPort] = true

	// Pick a shadow-tls listen port from snell common ports, excluding the snell port itself.
	stPort := 0
	for _, p := range protocol.CommonPorts(protocol.Snell) {
		if !used[p] {
			stPort = p
			break
		}
	}
	if stPort == 0 {
		stPort = protocol.DefaultPort(protocol.ShadowTLS, used)
	}

	params := protocol.InstallParams{
		ProtoType: protocol.ShadowTLS,
		Port:      stPort,
		UserName:  "user",
	}

	ctx := context.Background()
	depSteps := protocol.ProvisionDeps(ctx, protocol.ShadowTLS, params)

	// Filter out sing-box steps to avoid duplicate display (already shown during snell install).
	var stSteps []protocol.DepStep
	for _, s := range depSteps {
		if !strings.Contains(s.Description, "sing-box") {
			stSteps = append(stSteps, s)
		}
	}
	depReport := protocol.FormatDepSteps(stSteps)

	if protocol.HasDepError(depSteps) {
		return protoInstallDoneMsg{result: "shadow-tls 依赖安装失败\n\n" + protocol.FormatDepSteps(depSteps)}
	}

	msg := fmt.Sprintf("snell+shadow-tls 配置完成\nShadowTLS 监听: %d -> Snell 后端: %d",
		stPort, snellPort)
	if depReport != "" {
		msg += "\n" + depReport
	}
	return protoInstallDoneMsg{result: msg}
}
