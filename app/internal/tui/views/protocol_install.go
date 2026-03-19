package views

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-proxy/internal/protocol"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type ProtocolInstallView struct {
	tui.InlineState
	model       *tui.Model
	menu        tui.MenuModel
	split       tui.SubSplitModel
	step        protoInstallStep
	pendingType protocol.Type
	lastResult  *protocol.InstallResult
}

type protoInstallStep int

const (
	protoInstallMenu protoInstallStep = iota
	protoInstallPort
	protoInstallResult
	protoInstallShadowTLSPrompt
)

func NewProtocolInstallView(model *tui.Model) *ProtocolInstallView {
	return &ProtocolInstallView{model: model}
}

func (v *ProtocolInstallView) Name() string { return "protocol-install" }

func (v *ProtocolInstallView) Init() tea.Cmd {
	v.step = protoInstallMenu
	v.split.SetFocusLeft(true)
	v.split.SetSize(v.model.ContentWidth(), v.model.Height()-6)
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
		v.split.SetSize(msg.ContentWidth, msg.ContentHeight-6)
		return v, nil
	case tui.SubSplitMouseMsg:
		var cmd tea.Cmd
		v.split, cmd = v.split.Update(msg.MouseMsg)
		return v, cmd
	}
	inlineCmd, handled := v.UpdateInline(msg)
	if handled {
		return v, inlineCmd
	}
	switch msg := msg.(type) {
	case tui.MenuSelectMsg:
		v.pendingType = protocol.Type(msg.ID)
		defaultPort := v.computeDefaultPort(v.pendingType)
		v.step = protoInstallPort
		v.split.SetFocusLeft(false)
		return v, v.SetInline(components.NewTextInput("端口号:", fmt.Sprintf("%d", defaultPort)))

	case tui.InputResultMsg:
		if msg.Cancelled {
			v.step = protoInstallMenu
			v.split.SetFocusLeft(true)
			return v, nil
		}
		pt := v.pendingType
		portStr := msg.Value
		return v, tea.Batch(
			v.SetInline(components.NewSpinner("正在安装依赖...")),
			func() tea.Msg {
				return v.doInstall(pt, portStr)
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
			return v, v.SetInline(components.NewResult(fmt.Sprintf("安装 snell 端口 %d 成功\nPSK: %s",
				v.lastResult.Port, v.lastResult.Credential)))
		}

	case tui.ResultDismissedMsg:
		cmd := v.Init()
		return v, cmd

	default:
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyEsc {
			return v, tui.BackCmd
		}
		if v.step == protoInstallMenu {
			var cmd tea.Cmd
			v.menu, cmd = v.menu.Update(msg)
			return v, cmd
		}
	}
	return v, inlineCmd
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

func (v *ProtocolInstallView) doInstall(pt protocol.Type, portStr string) tea.Msg {
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	if port <= 0 || port > 65535 {
		return protoInstallDoneMsg{result: "端口号无效"}
	}

	params := protocol.InstallParams{
		ProtoType: pt,
		Port:      port,
		UserName:  "user",
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
			msg = "依赖安装完成\n\n" + depReport + "\n" + msg
		}
		return protoInstallDoneMsg{result: msg}
	}
	if err := v.model.Store().Apply(); err != nil {
		return protoInstallDoneMsg{result: "保存失败: " + err.Error()}
	}

	msg := fmt.Sprintf("安装 %s 端口 %d 成功\nTag: %s\nCredential: %s",
		pt, result.Port, result.Tag, result.Credential)
	if result.PublicKey != "" {
		msg += "\nPublic Key: " + result.PublicKey
	}
	if depReport != "" {
		msg = "依赖安装\n" + depReport + "\n" + msg
	}
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
