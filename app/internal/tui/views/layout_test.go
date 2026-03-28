package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"go-proxy/internal/config"
	"go-proxy/internal/store"
	"go-proxy/internal/tui"
	"go-proxy/internal/update"
)

func TestSelfUpdatePromptUsesSingleLine(t *testing.T) {
	model := tui.NewModel(&store.Store{}, "dev")
	view := NewSelfUpdateView(&model)
	view.Init()

	_, _ = view.Update(selfUpdateCheckDoneMsg{
		check: &update.SelfUpdateCheck{
			CurrentVersion: "v1.0.0",
			LatestVersion:  "v1.1.0",
			UpdateAvail:    true,
		},
	})

	if strings.Contains(view.confirmPrompt, "\n") {
		t.Fatalf("confirmPrompt = %q, want single line", view.confirmPrompt)
	}
	if !strings.Contains(view.confirmPrompt, "是否更新?") {
		t.Fatalf("confirmPrompt = %q, want update prompt", view.confirmPrompt)
	}
}

func TestServiceMenuShowsStatusFirst(t *testing.T) {
	model := tui.NewModel(&store.Store{}, "dev")
	view := NewServiceView(&model)

	menu := view.Menu.View()
	statusIndex := strings.Index(menu, "查看服务状态")
	restartIndex := strings.Index(menu, "重启所有服务")
	if statusIndex < 0 || restartIndex < 0 {
		t.Fatalf("menu = %q, want status and restart entries", menu)
	}
	if statusIndex > restartIndex {
		t.Fatalf("menu = %q, want 查看服务状态 before 重启所有服务", menu)
	}
}

func TestUninstallPreviewRendersTable(t *testing.T) {
	model := tui.NewModel(&store.Store{}, "dev")
	view := NewUninstallView(&model)
	view.Init()
	if !view.ready {
		t.Fatal("expected uninstall viewport to be ready after init")
	}

	if !strings.Contains(view.tableHeader, "类型") || !strings.Contains(view.tableHeader, "路径") {
		t.Fatalf("tableHeader = %q, want table headers", view.tableHeader)
	}
	if !strings.Contains(view.previewBody, config.WorkDir) {
		t.Fatalf("previewBody = %q, want work dir row", view.previewBody)
	}
	if !strings.Contains(view.tableHeader, "────") {
		t.Fatalf("tableHeader = %q, want separator lines", view.tableHeader)
	}
	if !strings.Contains(view.View(), view.confirmPrompt) {
		t.Fatalf("view = %q, want confirm prompt", view.View())
	}
	firstLine := strings.SplitN(view.View(), "\n", 2)[0]
	if !strings.Contains(firstLine, "将卸载以下服务、文件和配置") || !strings.Contains(firstLine, view.confirmPrompt) {
		t.Fatalf("first line = %q, want title and confirm prompt on same line", firstLine)
	}

	_, _ = view.Update(tui.ViewResizeMsg{ContentWidth: 60, ContentHeight: 8})
	_, _ = view.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if view.viewport.YOffset <= 0 {
		t.Fatalf("YOffset = %d, want scrollable preview", view.viewport.YOffset)
	}
}

func TestCoreUpdateOmitsResultsRow(t *testing.T) {
	model := tui.NewModel(&store.Store{}, "dev")
	view := NewCoreView(&model)
	msg := view.doUpdate(nil)
	done, ok := msg.(coreUpdateDoneMsg)
	if !ok {
		t.Fatalf("msg = %#v, want coreUpdateDoneMsg", msg)
	}
	if strings.Count(done.result, "结果") != 1 {
		t.Fatalf("result = %q, want exactly one results header", done.result)
	}
}

func TestWrapPanelContentWrapsLongLine(t *testing.T) {
	content := "1234567890ABCDEFGHIJ"
	got := wrapPanelContent(content, 8)
	if !strings.Contains(got, "\n") {
		t.Fatalf("wrapPanelContent() = %q, want wrapped content", got)
	}
}

func TestRenderTableWrapsCellsWithoutDroppingContent(t *testing.T) {
	got := renderTable(
		[]string{"实例", "路径"},
		[][]string{{"shadow-tls-sn ell-1443", "/etc/systemd/system/shadow-tls-snell-1443.service"}},
		28,
		false,
	)
	if !strings.Contains(got, "shadow-tls") || !strings.Contains(got, "systemd") {
		t.Fatalf("renderTable() = %q, want full cell content preserved", got)
	}
}

func TestRenderTableStacksManyColumnsWhenNarrow(t *testing.T) {
	got := renderTable(
		[]string{"#", "实例", "监听端口", "后端类型", "后端端口", "SNI", "密码"},
		[][]string{{"1", "shadow-tls-snell-1443", "8443", "snell", "1443", "www.example.com", "secret"}},
		56,
		false,
	)
	if !strings.Contains(got, "实例:") || !strings.Contains(got, "密码:") {
		t.Fatalf("renderTable() = %q, want stacked layout for narrow multi-column table", got)
	}
}
