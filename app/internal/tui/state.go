package tui

// ViewID identifies a TUI screen.
type ViewID int

const (
	ViewMainMenu ViewID = iota
	ViewProtocolInstall
	ViewProtocolRemove
	ViewUserMenu
	ViewRoutingMenu
	ViewServiceMenu
	ViewSubscription
	ViewConfig
	ViewLogs
	ViewCore
	ViewNetwork
	ViewSelfUpdate
	ViewUninstall
)

// NavStack manages view navigation with a stack-based model.
type NavStack struct {
	stack []ViewID
}

// NewNavStack creates a navigation stack starting at the main menu.
func NewNavStack() NavStack {
	return NavStack{stack: []ViewID{ViewMainMenu}}
}

// Current returns the current view.
func (n *NavStack) Current() ViewID {
	if len(n.stack) == 0 {
		return ViewMainMenu
	}
	return n.stack[len(n.stack)-1]
}

// Push navigates to a new view.
func (n *NavStack) Push(v ViewID) {
	n.stack = append(n.stack, v)
}

// Pop returns to the previous view. Returns false if already at root.
func (n *NavStack) Pop() bool {
	if len(n.stack) <= 1 {
		return false
	}
	n.stack = n.stack[:len(n.stack)-1]
	return true
}

// Reset clears the stack back to the main menu.
func (n *NavStack) Reset() {
	n.stack = []ViewID{ViewMainMenu}
}

// Depth returns the current stack depth.
func (n *NavStack) Depth() int {
	return len(n.stack)
}
