package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"go-proxy/internal/derived"
	"go-proxy/internal/protocol"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/components"
)

type ProtocolRemoveView struct {
	tui.InlineState
	model       *tui.Model
	menu        tui.MenuModel
	step        protoRemoveStep
	pendingTag  string
	tableHeader string
	emptyResult bool
}

type protoRemoveStep int

const (
	protoRemoveMenu protoRemoveStep = iota
	protoRemoveConfirm
	protoRemoveResult
)

func NewProtocolRemoveView(model *tui.Model) *ProtocolRemoveView {
	return &ProtocolRemoveView{model: model}
}

func (v *ProtocolRemoveView) Name() string { return "protocol-remove" }

func (v *ProtocolRemoveView) Init() tea.Cmd {
	v.step = protoRemoveMenu
	v.pendingTag = ""
	v.emptyResult = false
	// Reload store from disk to pick up changes from protocol install.
	v.model.Store().Reload()
	inv := derived.Inventory(v.model.Store())

	if len(inv) == 0 {
		v.step = protoRemoveResult
		v.emptyResult = true
		return v.SetInline(components.NewResult("没有已安装的协议"))
	}

	membership := derived.Membership(v.model.Store())

	// Build a reverse map: tag -> list of user names.
	tagUsers := make(map[string][]string)
	for name, entries := range membership {
		for _, e := range entries {
			tagUsers[e.Tag] = append(tagUsers[e.Tag], name)
		}
	}

	// Table header.
	v.tableHeader = fmt.Sprintf("  %-4s %-16s %-8s %-8s %s",
		"#", "协议", "端口", "用户", "详情")

	items := make([]tui.MenuItem, 0, len(inv)+1)
	for i, info := range inv {
		k := rune('1' + i)
		if i >= 9 {
			k = rune('a' + i - 9)
		}

		userCount := info.UserCount
		userDetail := strings.Join(tagUsers[info.Tag], "  ")
		if userDetail == "" {
			userDetail = "无"
		}

		detail := userDetail
		if info.HasReality {
			detail = "reality  " + detail
		}

		label := fmt.Sprintf("%-14s %-8d %-8d %s",
			info.Type, info.Port, userCount, detail)

		items = append(items, tui.MenuItem{
			Key:   k,
			Label: label,
			ID:    info.Tag,
		})
	}
	v.menu = v.menu.SetItems(items)
	return nil
}

func (v *ProtocolRemoveView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	inlineCmd, handled := v.UpdateInline(msg)
	if handled {
		return v, inlineCmd
	}
	switch msg := msg.(type) {
	case tui.MenuSelectMsg:
		// Validate that the ID is a real protocol tag.
		if derived.FindInbound(v.model.Store(), msg.ID) == nil {
			return v, nil
		}
		v.pendingTag = msg.ID
		v.step = protoRemoveConfirm
		prompt := fmt.Sprintf("确认卸载 %s?", v.pendingTag)
		return v, v.SetInline(components.NewConfirm(prompt))

	case tui.ConfirmResultMsg:
		if !msg.Confirmed {
			v.step = protoRemoveMenu
			return v, nil
		}
		tag := v.pendingTag
		return v, tea.Batch(
			v.SetInline(components.NewSpinner("正在卸载...")),
			func() tea.Msg {
				return protoRemoveDoneMsg{tag: tag, err: v.doRemove(tag)}
			},
		)

	case protoRemoveDoneMsg:
		v.step = protoRemoveResult
		var result string
		if msg.err != nil {
			result = "卸载失败: " + msg.err.Error()
		} else {
			result = "已卸载: " + msg.tag
		}
		return v, v.SetInline(components.NewResult(result))

	case tui.ResultDismissedMsg:
		if v.emptyResult {
			return v, tui.BackCmd
		}
		cmd := v.Init()
		return v, cmd

	default:
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyEsc {
			return v, tui.BackCmd
		}
		if v.step == protoRemoveMenu {
			var cmd tea.Cmd
			v.menu, cmd = v.menu.Update(msg)
			return v, cmd
		}
	}
	return v, inlineCmd
}

func (v *ProtocolRemoveView) View() string {
	if v.HasInline() {
		return v.ViewInline()
	}
	if v.step == protoRemoveMenu && v.tableHeader != "" {
		content := v.tableHeader + "\n" + v.menu.View()
		return tui.RenderSubMenuBody(content, v.model.ContentWidth())
	}
	return tui.RenderSubMenuBody(v.menu.View(), v.model.ContentWidth())
}

type protoRemoveDoneMsg struct {
	tag string
	err error
}

func (v *ProtocolRemoveView) doRemove(tag string) error {
	if err := protocol.Remove(v.model.Store(), tag); err != nil {
		return err
	}
	return v.model.Store().Apply()
}
