package routing

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dhwang2/go-proxy/internal/store"
)

// TestRouting prints a summary of routing rules for a given user.
// Returns per-outbound rule counts and total as a readable string.
func TestRouting(st *store.Store, user string) (string, error) {
	byOutbound, total, err := TestUser(st, user)
	if err != nil {
		return "", err
	}
	if total == 0 {
		return fmt.Sprintf("No routing rules found for user %q.", normalizeName(user)), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Routing summary for %q:\n", normalizeName(user))

	// Sort outbound names for deterministic output.
	outbounds := make([]string, 0, len(byOutbound))
	for ob := range byOutbound {
		outbounds = append(outbounds, ob)
	}
	sort.Strings(outbounds)

	for _, ob := range outbounds {
		fmt.Fprintf(&b, "  %-20s %d rule(s)\n", ob, byOutbound[ob])
	}
	fmt.Fprintf(&b, "  %-20s %d\n", "TOTAL", total)
	return b.String(), nil
}
