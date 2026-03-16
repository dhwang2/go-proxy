package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"go-proxy/internal/derived"
	"go-proxy/internal/protocol"
	"go-proxy/internal/routing"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
	"go-proxy/internal/subscription"
	"go-proxy/internal/tui/components"
	"go-proxy/internal/tui/views"
	"go-proxy/internal/user"
)

// App is the root Bubble Tea model.
type App struct {
	store   *store.Store
	nav     NavStack
	version string
	width   int
	height  int

	// Component state for the current view.
	menu    components.MenuModel
	input   components.InputModel
	confirm components.ConfirmModel
	spinner components.SpinnerModel
	toast   components.ToastModel

	// Transient state for multi-step flows.
	resultText string
	phase      int // step within a multi-step flow
	tempProto  protocol.Type
	tempTag    string
}

const phaseResult = 99

// applyChanges persists dirty store changes to disk.
// Returns an error string for display, or empty on success.
func (a *App) applyChanges() string {
	if err := a.store.Apply(); err != nil {
		return StyleError.Render("Failed to save: " + err.Error())
	}
	return ""
}

// New creates a new TUI application.
func New(s *store.Store, version string) App {
	app := App{
		store:   s,
		nav:     NewNavStack(),
		version: version,
	}
	app.menu = components.NewMenu("go-proxy "+version, views.MainMenuItems())
	return app
}

// Init implements tea.Model.
func (a App) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		return a, nil

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			if a.nav.Current() == ViewMainMenu {
				return a, tea.Quit
			}
		}
		if msg.Type == tea.KeyEsc {
			if a.nav.Pop() {
				a.resetView()
				return a, nil
			}
		}
		if msg.String() == "q" && a.nav.Current() == ViewMainMenu {
			return a, tea.Quit
		}

	case components.ToastExpiredMsg:
		a.toast, _ = a.toast.Update(msg)
		return a, nil
	}

	// Delegate to current view.
	switch a.nav.Current() {
	case ViewMainMenu:
		return a.updateMainMenu(msg)
	case ViewProtocolInstall:
		return a.updateProtocolInstall(msg)
	case ViewProtocolRemove:
		return a.updateProtocolRemove(msg)
	case ViewUserMenu:
		return a.updateUserMenu(msg)
	case ViewServiceMenu:
		return a.updateServiceMenu(msg)
	case ViewSubscription:
		return a.updateSubscription(msg)
	case ViewConfig:
		return a.updateConfig(msg)
	case ViewRoutingMenu:
		return a.updateRoutingMenu(msg)
	case ViewNetwork:
		return a.updateNetwork(msg)
	case ViewCore:
		return a.updateCore(msg)
	case ViewUninstall:
		return a.updateUninstall(msg)
	}

	return a, nil
}

// View implements tea.Model.
func (a App) View() string {
	var b strings.Builder

	switch a.nav.Current() {
	case ViewMainMenu:
		b.WriteString(a.viewMainMenu())
	case ViewProtocolInstall:
		b.WriteString(a.viewProtocolInstall())
	case ViewProtocolRemove:
		b.WriteString(a.viewProtocolRemove())
	case ViewUserMenu:
		b.WriteString(a.viewUserMenu())
	case ViewServiceMenu:
		b.WriteString(a.viewServiceMenu())
	case ViewSubscription:
		b.WriteString(a.viewSubscription())
	case ViewConfig:
		b.WriteString(a.viewConfig())
	case ViewRoutingMenu:
		b.WriteString(a.viewRoutingMenu())
	case ViewNetwork:
		b.WriteString(a.viewNetwork())
	case ViewCore:
		b.WriteString(a.viewCore())
	case ViewUninstall:
		b.WriteString(a.viewUninstall())
	default:
		b.WriteString(StyleMuted.Render("Not implemented yet"))
	}

	if a.toast.Visible() {
		b.WriteString("\n")
		b.WriteString(a.toast.View())
	}

	b.WriteString("\n")
	b.WriteString(StyleStatusBar.Render("ESC: back  q: quit"))

	return b.String()
}

// ---------- Main Menu ----------

func (a App) updateMainMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.menu, cmd = a.menu.Update(msg)

	if sel, ok := msg.(components.MenuSelectedMsg); ok {
		viewID := views.MainMenuViewID(sel.Index)
		if viewID == -1 {
			return a, tea.Quit
		}
		a.nav.Push(ViewID(viewID))
		a.resetView()
		return a, nil
	}
	return a, cmd
}

func (a App) viewMainMenu() string {
	var b strings.Builder
	// Dashboard header.
	stats := derived.Dashboard(a.store)
	header := fmt.Sprintf("Protocols: %d  Users: %d  Routes: %d",
		stats.ProtocolCount, stats.UserCount, stats.RouteCount)
	b.WriteString(StyleSubtitle.Render(header))
	b.WriteString("\n\n")
	b.WriteString(a.menu.View())
	return b.String()
}

// ---------- Protocol Install ----------

func (a App) updateProtocolInstall(msg tea.Msg) (tea.Model, tea.Cmd) {
	if a.phase == 0 {
		if a.menu.Title != "Select Protocol" {
			a.menu = components.NewMenu("Select Protocol", views.ProtocolInstallMenuItems())
		}
		var cmd tea.Cmd
		a.menu, cmd = a.menu.Update(msg)
		if sel, ok := msg.(components.MenuSelectedMsg); ok {
			types := protocol.AllTypes()
			if sel.Index >= len(types) {
				a.nav.Pop()
				a.resetView()
				return a, nil
			}
			a.tempProto = types[sel.Index]
			a.phase = 1
			a.input = components.NewInput("Enter port number:", "8443")
			return a, a.input.Init()
		}
		return a, cmd
	}
	if a.phase == 1 {
		var cmd tea.Cmd
		a.input, cmd = a.input.Update(msg)
		if sub, ok := msg.(components.InputSubmittedMsg); ok {
			port := 0
			fmt.Sscanf(sub.Value, "%d", &port)
			if port <= 0 || port > 65535 {
				a.resultText = StyleError.Render("Invalid port")
				a.phase = phaseResult
				return a, nil
			}
			result, err := protocol.Install(a.store, protocol.InstallParams{
				ProtoType: a.tempProto,
				Port:      port,
				UserName:  "user",
			})
			if err != nil {
				a.resultText = StyleError.Render("Error: " + err.Error())
			} else {
				if errMsg := a.applyChanges(); errMsg != "" {
					a.resultText = errMsg
				} else {
					a.resultText = StyleSuccess.Render(fmt.Sprintf(
						"Installed %s on port %d\nTag: %s\nCredential: %s",
						a.tempProto, result.Port, result.Tag, result.Credential))
					if result.PublicKey != "" {
						a.resultText += "\nPublic Key: " + result.PublicKey
					}
				}
			}
			a.phase = phaseResult
			return a, nil
		}
		return a, cmd
	}
	// phaseResult: showing result, any key goes back.
	if _, ok := msg.(tea.KeyMsg); ok {
		a.nav.Pop()
		a.resetView()
	}
	return a, nil
}

func (a App) viewProtocolInstall() string {
	switch a.phase {
	case 0:
		return a.menu.View()
	case 1:
		return a.input.View()
	default:
		return a.resultText
	}
}

// ---------- Protocol Remove ----------

func (a App) updateProtocolRemove(msg tea.Msg) (tea.Model, tea.Cmd) {
	if a.phase == 0 {
		inv := derived.Inventory(a.store)
		tags := make([]string, len(inv))
		for i, p := range inv {
			tags[i] = fmt.Sprintf("%s (port %d, %d users)", p.Tag, p.Port, p.UserCount)
		}
		if a.menu.Title != "Remove Protocol" {
			a.menu = components.NewMenu("Remove Protocol", views.ProtocolRemoveMenuItems(tags))
		}
		var cmd tea.Cmd
		a.menu, cmd = a.menu.Update(msg)
		if sel, ok := msg.(components.MenuSelectedMsg); ok {
			if sel.Index >= len(inv) {
				a.nav.Pop()
				a.resetView()
				return a, nil
			}
			a.tempTag = inv[sel.Index].Tag
			a.phase = 1
			a.confirm = components.NewConfirm(fmt.Sprintf("Remove %s?", a.tempTag))
			return a, nil
		}
		return a, cmd
	}
	if a.phase == 1 {
		var cmd tea.Cmd
		a.confirm, cmd = a.confirm.Update(msg)
		if res, ok := msg.(components.ConfirmResultMsg); ok {
			if res.Confirmed {
				if err := protocol.Remove(a.store, a.tempTag); err != nil {
					a.resultText = StyleError.Render("Error: " + err.Error())
				} else if errMsg := a.applyChanges(); errMsg != "" {
					a.resultText = errMsg
				} else {
					a.resultText = StyleSuccess.Render("Removed " + a.tempTag)
				}
			} else {
				a.resultText = StyleMuted.Render("Cancelled")
			}
			a.phase = phaseResult
			return a, nil
		}
		return a, cmd
	}
	if _, ok := msg.(tea.KeyMsg); ok {
		a.nav.Pop()
		a.resetView()
	}
	return a, nil
}

func (a App) viewProtocolRemove() string {
	switch a.phase {
	case 0:
		return a.menu.View()
	case 1:
		return a.confirm.View()
	default:
		return a.resultText
	}
}

// ---------- User Menu ----------

func (a App) updateUserMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	if a.phase == 0 {
		if a.menu.Title != "User Management" {
			a.menu = components.NewMenu("User Management", views.UserMenuItems())
		}
		var cmd tea.Cmd
		a.menu, cmd = a.menu.Update(msg)
		if sel, ok := msg.(components.MenuSelectedMsg); ok {
			switch sel.Index {
			case 0: // List
				users := user.List(a.store)
				var sb strings.Builder
				sb.WriteString(StyleTitle.Render("Users"))
				sb.WriteString("\n\n")
				if len(users) == 0 {
					sb.WriteString(StyleMuted.Render("No users found"))
				}
				for _, u := range users {
					sb.WriteString(fmt.Sprintf("  %s  (%d protocols, %d routes)\n",
						StyleAccent.Render(u.Name), len(u.Memberships), u.RouteCount))
				}
				a.resultText = sb.String()
				a.phase = phaseResult
			case 1: // Add
				a.phase = 10
				a.input = components.NewInput("Enter username:", "user2")
				return a, a.input.Init()
			case 2: // Rename
				a.phase = 20
				a.input = components.NewInput("Enter current username:", "")
				return a, a.input.Init()
			case 3: // Delete
				a.phase = 30
				a.input = components.NewInput("Enter username to delete:", "")
				return a, a.input.Init()
			case 4: // Back
				a.nav.Pop()
				a.resetView()
			}
			return a, nil
		}
		return a, cmd
	}

	switch a.phase {
	case 10: // Add user - input name
		var cmd tea.Cmd
		a.input, cmd = a.input.Update(msg)
		if sub, ok := msg.(components.InputSubmittedMsg); ok {
			if err := user.Add(a.store, sub.Value); err != nil {
				a.resultText = StyleError.Render("Error: " + err.Error())
			} else if errMsg := a.applyChanges(); errMsg != "" {
				a.resultText = errMsg
			} else {
				a.resultText = StyleSuccess.Render("Added user: " + sub.Value)
			}
			a.phase = phaseResult
		}
		return a, cmd
	case 20: // Rename - old name
		var cmd tea.Cmd
		a.input, cmd = a.input.Update(msg)
		if sub, ok := msg.(components.InputSubmittedMsg); ok {
			a.tempTag = sub.Value
			a.phase = 21
			a.input = components.NewInput("Enter new username:", "")
			return a, a.input.Init()
		}
		return a, cmd
	case 21: // Rename - new name
		var cmd tea.Cmd
		a.input, cmd = a.input.Update(msg)
		if sub, ok := msg.(components.InputSubmittedMsg); ok {
			if err := user.Rename(a.store, a.tempTag, sub.Value); err != nil {
				a.resultText = StyleError.Render("Error: " + err.Error())
			} else if errMsg := a.applyChanges(); errMsg != "" {
				a.resultText = errMsg
			} else {
				a.resultText = StyleSuccess.Render(fmt.Sprintf("Renamed %s → %s", a.tempTag, sub.Value))
			}
			a.phase = phaseResult
		}
		return a, cmd
	case 30: // Delete user
		var cmd tea.Cmd
		a.input, cmd = a.input.Update(msg)
		if sub, ok := msg.(components.InputSubmittedMsg); ok {
			if err := user.Delete(a.store, sub.Value); err != nil {
				a.resultText = StyleError.Render("Error: " + err.Error())
			} else if errMsg := a.applyChanges(); errMsg != "" {
				a.resultText = errMsg
			} else {
				a.resultText = StyleSuccess.Render("Deleted user: " + sub.Value)
			}
			a.phase = phaseResult
		}
		return a, cmd
	}

	// Phase 99: result display.
	if _, ok := msg.(tea.KeyMsg); ok {
		a.phase = 0
		a.menu.ResetSelection()
	}
	return a, nil
}

func (a App) viewUserMenu() string {
	switch {
	case a.phase == 0:
		return a.menu.View()
	case a.phase >= 10 && a.phase < phaseResult:
		return a.input.View()
	default:
		return a.resultText
	}
}

// ---------- Service Menu ----------

func (a App) updateServiceMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	if a.phase == 0 {
		if a.menu.Title != "Service Management" {
			a.menu = components.NewMenu("Service Management", views.ServiceMenuItems())
		}
		var cmd tea.Cmd
		a.menu, cmd = a.menu.Update(msg)
		if sel, ok := msg.(components.MenuSelectedMsg); ok {
			svcs := service.AllServices()
			if sel.Index >= len(svcs) {
				a.nav.Pop()
				a.resetView()
				return a, nil
			}
			a.tempTag = string(svcs[sel.Index])
			a.phase = 1
			a.menu = components.NewMenu(a.tempTag, views.ServiceActionItems())
			return a, nil
		}
		return a, cmd
	}
	if a.phase == 1 {
		var cmd tea.Cmd
		a.menu, cmd = a.menu.Update(msg)
		if sel, ok := msg.(components.MenuSelectedMsg); ok {
			ctx := context.Background()
			svc := service.Name(a.tempTag)
			switch sel.Index {
			case 0:
				err := service.Start(ctx, svc)
				a.resultText = actionResult("Start", a.tempTag, err)
			case 1:
				err := service.Stop(ctx, svc)
				a.resultText = actionResult("Stop", a.tempTag, err)
			case 2:
				err := service.Restart(ctx, svc)
				a.resultText = actionResult("Restart", a.tempTag, err)
			case 3:
				st, err := service.GetStatus(ctx, svc)
				if err != nil {
					a.resultText = StyleError.Render("Error: " + err.Error())
				} else {
					status := "inactive"
					if st.Running {
						status = "active (running)"
					}
					a.resultText = fmt.Sprintf("%s: %s", a.tempTag, StyleAccent.Render(status))
				}
			case 4:
				a.phase = 0
				a.menu = components.NewMenu("Service Management", views.ServiceMenuItems())
				return a, nil
			}
			a.phase = phaseResult
			return a, nil
		}
		return a, cmd
	}
	if _, ok := msg.(tea.KeyMsg); ok {
		a.phase = 0
		a.menu = components.NewMenu("Service Management", views.ServiceMenuItems())
	}
	return a, nil
}

func (a App) viewServiceMenu() string {
	if a.phase < phaseResult {
		return a.menu.View()
	}
	return a.resultText
}

// ---------- Subscription ----------

func (a App) updateSubscription(msg tea.Msg) (tea.Model, tea.Cmd) {
	if a.phase == 0 {
		// Select user.
		names := derived.UserNames(a.store)
		if len(names) == 0 {
			a.resultText = StyleMuted.Render("No users found")
			a.phase = phaseResult
			return a, nil
		}
		items := make([]components.MenuItem, len(names)+1)
		for i, n := range names {
			items[i] = components.MenuItem{Key: fmt.Sprintf("%d", i+1), Label: n}
		}
		items[len(names)] = components.MenuItem{Key: "0", Label: "返回  Back"}
		if a.menu.Title != "Select User" {
			a.menu = components.NewMenu("Select User", items)
		}
		var cmd tea.Cmd
		a.menu, cmd = a.menu.Update(msg)
		if sel, ok := msg.(components.MenuSelectedMsg); ok {
			if sel.Index >= len(names) {
				a.nav.Pop()
				a.resetView()
				return a, nil
			}
			a.tempTag = names[sel.Index]
			a.phase = 1
			a.menu = components.NewMenu("Select Format", views.SubscriptionFormatItems())
			return a, nil
		}
		return a, cmd
	}
	if a.phase == 1 {
		var cmd tea.Cmd
		a.menu, cmd = a.menu.Update(msg)
		if sel, ok := msg.(components.MenuSelectedMsg); ok {
			var format subscription.Format
			switch sel.Index {
			case 0:
				format = subscription.FormatSurge
			case 1:
				format = subscription.FormatSingBox
			default:
				a.phase = 0
				return a, nil
			}
			links := subscription.Render(a.store, a.tempTag, format, "")
			var sb strings.Builder
			sb.WriteString(StyleTitle.Render(fmt.Sprintf("Subscription: %s (%s)", a.tempTag, format)))
			sb.WriteString("\n\n")
			for _, l := range links {
				sb.WriteString(StyleAccent.Render(l.Tag))
				sb.WriteString("\n")
				sb.WriteString(l.Content)
				sb.WriteString("\n\n")
			}
			if len(links) == 0 {
				sb.WriteString(StyleMuted.Render("No subscriptions available"))
			}
			a.resultText = sb.String()
			a.phase = phaseResult
			return a, nil
		}
		return a, cmd
	}
	if _, ok := msg.(tea.KeyMsg); ok {
		a.nav.Pop()
		a.resetView()
	}
	return a, nil
}

func (a App) viewSubscription() string {
	if a.phase < phaseResult {
		return a.menu.View()
	}
	return a.resultText
}

// ---------- Config View ----------

func (a App) updateConfig(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		a.nav.Pop()
		a.resetView()
	}
	return a, nil
}

func (a App) viewConfig() string {
	return StyleTitle.Render("sing-box Configuration") + "\n\n" + views.RenderConfig(a.store)
}

// ---------- Routing Menu ----------

func (a App) updateRoutingMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	if a.phase == 0 {
		if a.menu.Title != "Routing Management" {
			a.menu = components.NewMenu("Routing Management", views.RoutingMenuItems())
		}
		var cmd tea.Cmd
		a.menu, cmd = a.menu.Update(msg)
		if sel, ok := msg.(components.MenuSelectedMsg); ok {
			switch sel.Index {
			case 4: // Sync DNS
				routing.SyncDNS(a.store, nil, "ipv4_only")
				routing.SyncRouteRules(a.store)
				if errMsg := a.applyChanges(); errMsg != "" {
					a.resultText = errMsg
				} else {
					a.resultText = StyleSuccess.Render("DNS and route rules synced")
				}
				a.phase = phaseResult
			case 5: // Back
				a.nav.Pop()
				a.resetView()
			default:
				a.resultText = StyleMuted.Render("Not implemented yet")
				a.phase = phaseResult
			}
			return a, nil
		}
		return a, cmd
	}
	if _, ok := msg.(tea.KeyMsg); ok {
		a.phase = 0
		a.menu.ResetSelection()
	}
	return a, nil
}

func (a App) viewRoutingMenu() string {
	if a.phase < phaseResult {
		return a.menu.View()
	}
	return a.resultText
}

// ---------- Network ----------

func (a App) updateNetwork(msg tea.Msg) (tea.Model, tea.Cmd) {
	if a.phase == 0 {
		if a.menu.Title != "Network Management" {
			a.menu = components.NewMenu("Network Management", views.NetworkMenuItems())
		}
		var cmd tea.Cmd
		a.menu, cmd = a.menu.Update(msg)
		if sel, ok := msg.(components.MenuSelectedMsg); ok {
			if sel.Index == len(views.NetworkMenuItems())-1 {
				a.nav.Pop()
				a.resetView()
				return a, nil
			}
			a.resultText = StyleMuted.Render("Not implemented yet (requires Linux)")
			a.phase = phaseResult
			return a, nil
		}
		return a, cmd
	}
	if _, ok := msg.(tea.KeyMsg); ok {
		a.phase = 0
		a.menu.ResetSelection()
	}
	return a, nil
}

func (a App) viewNetwork() string {
	if a.phase < phaseResult {
		return a.menu.View()
	}
	return a.resultText
}

// ---------- Core ----------

func (a App) updateCore(msg tea.Msg) (tea.Model, tea.Cmd) {
	if a.phase == 0 {
		if a.menu.Title != "Core Management" {
			a.menu = components.NewMenu("Core Management", views.CoreMenuItems())
		}
		var cmd tea.Cmd
		a.menu, cmd = a.menu.Update(msg)
		if sel, ok := msg.(components.MenuSelectedMsg); ok {
			if sel.Index == len(views.CoreMenuItems())-1 {
				a.nav.Pop()
				a.resetView()
				return a, nil
			}
			a.resultText = StyleMuted.Render("Not implemented yet")
			a.phase = phaseResult
			return a, nil
		}
		return a, cmd
	}
	if _, ok := msg.(tea.KeyMsg); ok {
		a.phase = 0
		a.menu.ResetSelection()
	}
	return a, nil
}

func (a App) viewCore() string {
	if a.phase < phaseResult {
		return a.menu.View()
	}
	return a.resultText
}

// ---------- Uninstall ----------

func (a App) updateUninstall(msg tea.Msg) (tea.Model, tea.Cmd) {
	if a.phase == 0 {
		a.confirm = components.NewConfirm("Uninstall all services and configuration?")
		a.phase = 1
		return a, nil
	}
	if a.phase == 1 {
		var cmd tea.Cmd
		a.confirm, cmd = a.confirm.Update(msg)
		if res, ok := msg.(components.ConfirmResultMsg); ok {
			if res.Confirmed {
				ctx := context.Background()
				if err := service.Uninstall(ctx); err != nil {
					a.resultText = StyleError.Render("Uninstall error: " + err.Error())
				} else {
					a.resultText = StyleSuccess.Render("Uninstalled successfully")
				}
			} else {
				a.resultText = StyleMuted.Render("Cancelled")
			}
			a.phase = phaseResult
			return a, nil
		}
		return a, cmd
	}
	if _, ok := msg.(tea.KeyMsg); ok {
		return a, tea.Quit
	}
	return a, nil
}

func (a App) viewUninstall() string {
	if a.phase == 1 {
		return a.confirm.View()
	}
	return a.resultText
}

// ---------- Helpers ----------

func (a *App) resetView() {
	a.phase = 0
	a.resultText = ""
	a.tempProto = ""
	a.tempTag = ""
	// Reset menu to main menu if back at root.
	if a.nav.Current() == ViewMainMenu {
		a.menu = components.NewMenu("go-proxy "+a.version, views.MainMenuItems())
	}
}

func actionResult(action, target string, err error) string {
	if err != nil {
		return StyleError.Render(fmt.Sprintf("%s %s failed: %s", action, target, err))
	}
	return StyleSuccess.Render(fmt.Sprintf("%s %s: OK", action, target))
}
