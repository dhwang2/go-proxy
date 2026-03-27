package views

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/cert"
	"go-proxy/internal/crypto"
	"go-proxy/internal/derived"
	"go-proxy/internal/protocol"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type ProtocolInstallView struct {
	tui.SplitViewBase
	step          protoInstallStep
	pendingType   protocol.Type
	pendingUser   string
	pendingPort   int
	pendingDomain string
	pendingEmail  string
	lastResult    *protocol.InstallResult
}

type protoInstallStep int

const (
	protoInstallMenu protoInstallStep = iota
	protoInstallUser
	protoInstallPort
	protoInstallDomain
	protoInstallEmail
	protoInstallCert
	protoInstallResult
	protoInstallShadowTLSPrompt
)

func NewProtocolInstallView(model *tui.Model) *ProtocolInstallView {
	v := &ProtocolInstallView{}
	v.Model = model
	return v
}

func (v *ProtocolInstallView) Name() string { return "protocol-install" }

func (v *ProtocolInstallView) setFocus(left bool) {
	v.SetFocus(left)
}

func (v *ProtocolInstallView) Init() tea.Cmd {
	v.ClearInline()
	v.resetMenuState(v.Model.ContentWidth(), v.Model.Height()-5)
	return nil
}

func (v *ProtocolInstallView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.ViewResizeMsg:
		v.HandleResize(msg)
		return v, nil
	case tui.SubSplitMouseMsg:
		return v, v.HandleMouse(msg)
	}
	// In split mode, intercept up/down for menu navigation even when content is showing.
	if cmd, handled := v.HandleMenuNav(msg, v.step == protoInstallMenu, false); handled {
		return v, cmd
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
		v.SetFocus(false)
		return v, v.triggerMenuAction(msg.ID)

	case tui.InputResultMsg:
		if msg.Cancelled {
			v.step = protoInstallMenu
			v.SetFocus(true)
			return v, nil
		}
		switch v.step {
		case protoInstallUser:
			return v.handleUserSelect(msg.Value)
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
			return v, v.SetInline(components.NewResult("证书失败: " + msg.err.Error()))
		}
		// Cert done — proceed to install with cached port.
		v.step = protoInstallResult
		port := v.pendingPort
		pt := v.pendingType
		return v, tea.Batch(
			v.SetInline(components.NewSpinner("安装中...")),
			func() tea.Msg {
				return v.doInstallWithPort(pt, port)
			},
		)

	case protoInstallDoneMsg:
		v.step = protoInstallResult
		v.lastResult = msg.installResult
		if msg.installResult != nil && v.shouldPromptShadowTLS(v.pendingType) {
			v.step = protoInstallShadowTLSPrompt
			resultText := msg.result + "\n\n是否配置 shadow-tls 保护此端口?"
			return v, v.SetInline(components.NewConfirm(resultText))
		}
		return v, v.SetInline(components.NewResult(msg.result))

	case tui.ConfirmResultMsg:
		if v.step == protoInstallShadowTLSPrompt {
			if msg.Confirmed && v.lastResult != nil {
				backendPort := v.lastResult.Port
				backendType := v.shadowTLSBackendType(v.pendingType)
				return v, tea.Batch(
					v.SetInline(components.NewSpinner("配置 shadow-tls...")),
					func() tea.Msg {
						return v.doShadowTLSForBackend(backendType, backendPort)
					},
				)
			}
			// User declined; show the original result.
			v.step = protoInstallResult
			return v, v.SetInline(components.NewResult(formatInstallSuccess(v.pendingType, v.lastResult, v.pendingDomain != "")))
		}

	case tui.ResultDismissedMsg:
		v.resetMenuState(v.Split.TotalWidth(), v.Split.TotalHeight())
		return v, nil

	default:
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyEsc {
			return v, tui.BackCmd
		}
		// Left/Right arrow toggles sub-split focus.
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if v.HandleSplitArrows(keyMsg, v.step == protoInstallMenu, v.HasInline()) {
				return v, nil
			}
		}
		if v.step == protoInstallMenu {
			var cmd tea.Cmd
			v.Menu, cmd = v.Menu.Update(msg)
			return v, cmd
		}
	}
	return v, inlineCmd
}

func (v *ProtocolInstallView) View() string {
	if v.step == protoInstallMenu || !v.Split.Enabled() {
		if v.HasInline() {
			return v.ViewInline()
		}
		return tui.RenderSubMenuBody(v.Menu.View(), v.Model.ContentWidth())
	}

	menuContent := v.Menu.View()
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

	return v.Split.View(menuContent, detailContent)
}

// triggerMenuAction executes the action for the given menu item ID.
// Selects user first, then port (shell-proxy order).
func (v *ProtocolInstallView) triggerMenuAction(id string) tea.Cmd {
	v.pendingType = protocol.Type(id)
	v.pendingUser = ""
	v.pendingPort = 0
	v.pendingDomain = ""
	v.pendingEmail = ""
	names := derived.UserNames(v.Model.Store())
	if len(names) == 0 {
		return v.SetInline(components.NewResult("请先添加用户"))
	}
	if len(names) == 1 {
		v.pendingUser = names[0]
		return v.startInstallOrAddUser()
	}
	v.step = protoInstallUser
	return v.SetInline(components.NewSelectList("选择用户:", names))
}

func (v *ProtocolInstallView) resetMenuState(contentWidth, contentHeight int) {
	v.step = protoInstallMenu
	v.pendingType = ""
	v.pendingUser = ""
	v.pendingPort = 0
	v.pendingDomain = ""
	v.pendingEmail = ""
	v.lastResult = nil
	v.SetFocus(true)
	// Protocol install needs a narrower third-level split so the user picker
	// still renders beside the protocol list on common terminal widths and
	// long protocol labels do not wrap.
	v.Split.SetMinWidths(14, 10)
	if contentWidth <= 0 {
		contentWidth = v.Model.ContentWidth()
	}
	if contentHeight <= 0 {
		contentHeight = v.Model.Height() - 5
	}
	v.Split.SetSize(contentWidth, contentHeight)

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
	v.Menu = v.Menu.SetItems(items)
}

// startInstallOrAddUser checks for an existing inbound; if found, adds user directly.
// Otherwise proceeds to the port input flow.
func (v *ProtocolInstallView) startInstallOrAddUser() tea.Cmd {
	pt := v.pendingType
	userName := v.pendingUser

	// Snell is single-user; block duplicate install.
	if pt == protocol.Snell {
		if v.Model.Store().SnellConf != nil {
			if userHasSelectedProtocol(v.Model.Store(), pt, userName) {
				v.step = protoInstallResult
				return v.SetInline(components.NewResult(formatAlreadyInstalled(pt, userName)))
			}
			v.step = protoInstallResult
			return v.SetInline(components.NewResult("Snell 已安装，不支持多用户"))
		}
		return v.startPortInput()
	}

	existing := protocol.FindExistingInbound(v.Model.Store(), pt)
	if existing != nil {
		if userHasSelectedProtocol(v.Model.Store(), pt, userName) {
			v.step = protoInstallResult
			return v.SetInline(components.NewResult(formatAlreadyInstalled(pt, userName)))
		}
		v.step = protoInstallResult
		return tea.Batch(
			v.SetInline(components.NewSpinner("添加用户...")),
			func() tea.Msg {
				result, err := protocol.AddUserToExisting(v.Model.Store(), existing, userName)
				if err != nil {
					return protoInstallDoneMsg{result: "添加失败: " + err.Error()}
				}
				if err := v.Model.Store().Apply(); err != nil {
					return protoInstallDoneMsg{result: "保存失败: " + err.Error()}
				}
				// Restart sing-box to load the updated user list.
				_ = service.Restart(context.Background(), service.SingBox)
				return protoInstallDoneMsg{
					result:        formatAddUserSuccess(pt, result),
					installResult: result,
				}
			},
		)
	}
	return v.startPortInput()
}

// startPortInput transitions to the port input step.
func (v *ProtocolInstallView) startPortInput() tea.Cmd {
	defaultPort := v.computeDefaultPort(v.pendingType)
	v.step = protoInstallPort
	return v.SetInline(components.NewTextInput("端口号:", fmt.Sprintf("%d", defaultPort)))
}

// handleUserSelect processes the selected user and decides next step.
func (v *ProtocolInstallView) handleUserSelect(name string) (tui.View, tea.Cmd) {
	v.pendingUser = name
	return v, v.startInstallOrAddUser()
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
		v.SetInline(components.NewSpinner("安装中...")),
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
			v.SetInline(components.NewSpinner("安装中...")),
			func() tea.Msg {
				return v.doInstallWithPort(pt, port)
			},
		)
	}
	// Need to issue cert — ask for email first.
	v.step = protoInstallEmail
	return v, v.SetInline(components.NewTextInput("邮箱 (用于 Let's Encrypt):", cert.DefaultEmail(domain)))
}

// handleEmailInput processes the submitted email and starts cert issuance.
func (v *ProtocolInstallView) handleEmailInput(email string) (tui.View, tea.Cmd) {
	v.pendingEmail = email
	v.step = protoInstallCert
	d := v.pendingDomain
	e := email
	return v, tea.Batch(
		v.SetInline(components.NewTimedSpinner("等待证书签发...", 300)),
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
	for _, ib := range v.Model.Store().SingBox.Inbounds {
		ports = append(ports, ib.ListenPort)
	}
	if v.Model.Store().SnellConf != nil {
		if p := v.Model.Store().SnellConf.Port(); p > 0 {
			ports = append(ports, p)
		}
	}
	if bindings, err := service.ListShadowTLSBindings(v.Model.Store()); err == nil {
		for _, binding := range bindings {
			ports = append(ports, binding.ListenPort)
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
		UserName:  v.pendingUser,
		Domain:    v.pendingDomain,
	}

	// Provision dependencies (download binaries, create systemd services).
	ctx := context.Background()
	depSteps := protocol.ProvisionDeps(ctx, pt, params)
	depReport := protocol.FormatDepSteps(depSteps)

	if protocol.HasDepError(depSteps) {
		msg := "依赖失败\n\n" + depReport
		return protoInstallDoneMsg{result: msg}
	}

	// Install protocol configuration.
	result, err := protocol.Install(v.Model.Store(), params)
	if err != nil {
		msg := "安装失败: " + err.Error()
		if depReport != "" {
			msg = depReport + "\n" + msg
		}
		return protoInstallDoneMsg{result: msg}
	}
	if err := v.Model.Store().Apply(); err != nil {
		return protoInstallDoneMsg{result: "保存失败: " + err.Error()}
	}
	return protoInstallDoneMsg{
		result:        formatInstallSuccess(pt, result, v.pendingDomain != ""),
		installResult: result,
	}
}

func (v *ProtocolInstallView) doShadowTLSForBackend(backendType string, backendPort int) tea.Msg {
	used := v.collectUsedPorts()
	used[backendPort] = true

	ctx := context.Background()
	depSteps := protocol.ProvisionDeps(ctx, protocol.ShadowTLS, protocol.InstallParams{
		ProtoType: protocol.ShadowTLS,
	})
	if protocol.HasDepError(depSteps) {
		return protoInstallDoneMsg{result: "shadow-tls 依赖安装失败\n\n" + protocol.FormatDepSteps(depSteps)}
	}

	binding, err := service.FindShadowTLSBindingByBackend(v.Model.Store(), backendType, backendPort)
	if err != nil {
		return protoInstallDoneMsg{result: "shadow-tls 读取失败: " + err.Error()}
	}

	stPort := 0
	if binding != nil && binding.ListenPort > 0 {
		stPort = binding.ListenPort
		delete(used, stPort)
	} else {
		for _, p := range protocol.CommonPorts(v.pendingType) {
			if !used[p] {
				stPort = p
				break
			}
		}
		if stPort == 0 {
			stPort = protocol.DefaultPort(protocol.ShadowTLS, used)
		}
	}

	password := ""
	if binding != nil {
		password = binding.Password
	}
	if password == "" {
		password, err = crypto.GeneratePassword(16)
		if err != nil {
			return protoInstallDoneMsg{result: "shadow-tls 配置失败: " + err.Error()}
		}
	}

	sni := ""
	if binding != nil {
		sni = binding.SNI
	}
	if sni == "" {
		sni = v.pendingDomain
	}
	if sni == "" {
		sni = protocol.DetectTLSDomain()
	}
	if sni == "" {
		sni = "www.microsoft.com"
	}

	serviceName, err := service.ProvisionShadowTLSBinding(ctx, backendType, stPort, password, sni, backendPort)
	if err != nil {
		return protoInstallDoneMsg{result: "shadow-tls 配置失败: " + err.Error()}
	}

	shadowTLSService := service.Name(serviceName)
	if err := service.Enable(ctx, shadowTLSService); err != nil {
		return protoInstallDoneMsg{result: "shadow-tls 启用失败: " + err.Error()}
	}
	if err := service.Restart(ctx, shadowTLSService); err != nil {
		return protoInstallDoneMsg{result: "shadow-tls 重启失败: " + err.Error()}
	}

	msg := fmt.Sprintf("shadow-tls 已配置\n监听: %d\n后端: %s:%d", stPort, backendType, backendPort)
	return protoInstallDoneMsg{result: msg}
}

func (v *ProtocolInstallView) shouldPromptShadowTLS(pt protocol.Type) bool {
	return pt == protocol.Snell || pt == protocol.Shadowsocks
}

func (v *ProtocolInstallView) shadowTLSBackendType(pt protocol.Type) string {
	if pt == protocol.Snell {
		return "snell"
	}
	return "ss"
}

func formatInstallSuccess(pt protocol.Type, result *protocol.InstallResult, withCert bool) string {
	if result == nil {
		return "安装完成"
	}
	label := string(pt)
	if spec, ok := protocol.Specs()[pt]; ok && spec.DisplayName != "" {
		label = spec.DisplayName
	}
	lines := []string{
		"安装完成",
		"协议: " + label,
		fmt.Sprintf("端口: %d", result.Port),
	}
	if withCert {
		lines = append(lines, "证书: ok")
	}
	if result.Credential != "" {
		lines = append(lines, "凭据: "+result.Credential)
	}
	if result.PublicKey != "" {
		lines = append(lines, "公钥: "+result.PublicKey)
	}
	return strings.Join(lines, "\n")
}

func formatAddUserSuccess(pt protocol.Type, result *protocol.InstallResult) string {
	if result == nil {
		return "添加完成"
	}
	label := string(pt)
	if spec, ok := protocol.Specs()[pt]; ok && spec.DisplayName != "" {
		label = spec.DisplayName
	}
	lines := []string{
		"添加完成",
		"协议: " + label,
		fmt.Sprintf("端口: %d", result.Port),
	}
	if result.Credential != "" {
		lines = append(lines, "凭据: "+result.Credential)
	}
	return strings.Join(lines, "\n")
}

func formatAlreadyInstalled(pt protocol.Type, userName string) string {
	label := string(pt)
	if spec, ok := protocol.Specs()[pt]; ok && spec.DisplayName != "" {
		label = spec.DisplayName
	}
	return fmt.Sprintf("无需重复安装\n用户: %s\n协议: %s", userName, label)
}

func userHasSelectedProtocol(s *store.Store, pt protocol.Type, userName string) bool {
	if s == nil || userName == "" {
		return false
	}
	if pt == protocol.Snell {
		return s.SnellConf != nil && derived.Membership(s)[userName] != nil && hasProtocolMembership(derived.Membership(s)[userName], store.SnellTag)
	}
	spec, ok := protocol.Specs()[pt]
	if !ok || spec.SingBoxType == "" {
		return false
	}
	for _, ib := range s.SingBox.Inbounds {
		if ib.Type != spec.SingBoxType {
			continue
		}
		ibHasReality := ib.TLS != nil && ib.TLS.Reality != nil && ib.TLS.Reality.Enabled
		if ibHasReality != spec.UsesReality {
			continue
		}
		if ib.FindUser(userName) != nil {
			return true
		}
	}
	return false
}

func hasProtocolMembership(entries []derived.MembershipEntry, proto string) bool {
	for _, entry := range entries {
		if entry.Proto == proto || entry.Tag == proto {
			return true
		}
	}
	return false
}
