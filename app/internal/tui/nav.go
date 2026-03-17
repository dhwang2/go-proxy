package tui

// NavState manages stack-based page navigation.
type NavState struct {
	stack []string
}

// Push adds a page name to the navigation stack.
func (n *NavState) Push(name string) {
	n.stack = append(n.stack, name)
}

// Pop removes and returns the top page, returning to the previous one.
// Returns the new top page name. If only one page remains, returns it unchanged.
func (n *NavState) Pop() string {
	if len(n.stack) <= 1 {
		return n.Current()
	}
	n.stack = n.stack[:len(n.stack)-1]
	return n.stack[len(n.stack)-1]
}

// Current returns the top page name on the stack.
func (n *NavState) Current() string {
	if len(n.stack) == 0 {
		return ""
	}
	return n.stack[len(n.stack)-1]
}

// Depth returns the number of pages on the stack.
func (n *NavState) Depth() int {
	return len(n.stack)
}

// Breadcrumb returns all page names in order for breadcrumb rendering.
func (n *NavState) Breadcrumb() []string {
	result := make([]string, len(n.stack))
	copy(result, n.stack)
	return result
}

// Clear removes all entries from the stack.
func (n *NavState) Clear() {
	n.stack = n.stack[:0]
}
