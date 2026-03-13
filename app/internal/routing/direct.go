package routing

import (
	"fmt"
	"strings"

	"github.com/dhwang2/go-proxy/internal/store"
)

// SetDirectMode configures direct outbound for a user, removing all routing rules.
func SetDirectMode(st *store.Store, user string) (MutationResult, error) {
	if st == nil || st.Config == nil || st.UserMeta == nil {
		return MutationResult{}, fmt.Errorf("store is nil")
	}
	user = normalizeName(user)
	if user == "" {
		return MutationResult{}, fmt.Errorf("usage: routing direct <user>")
	}
	if _, ok := st.UserMeta.Groups[user]; !ok {
		return MutationResult{}, fmt.Errorf("group not found: %s", user)
	}

	// Clear all existing route rules for the user.
	next := make([]store.RouteRule, 0, len(st.Config.Route.Rules)+1)
	removed := 0
	for _, rule := range st.Config.Route.Rules {
		if strings.TrimSpace(rule.Action) != "route" || len(rule.RuleSet) == 0 {
			next = append(next, rule)
			continue
		}
		filtered := removeUser(rule.AuthUser, user)
		if len(filtered) == 0 && containsUser(rule.AuthUser, user) {
			removed++
			continue
		}
		if len(filtered) != len(rule.AuthUser) {
			rule.AuthUser = filtered
			removed++
		}
		next = append(next, rule)
	}

	// Add a single direct outbound rule for the user.
	next = append(next, store.RouteRule{
		Action:   "route",
		Outbound: "direct",
		AuthUser: []string{user},
	})
	st.Config.Route.Rules = next

	dnsChanged := rebuildManagedDNS(st)
	st.MarkConfigDirty()
	return MutationResult{
		ConfigChanged: true,
		RouteChanged:  removed + 1,
		DNSChanged:    dnsChanged,
	}, nil
}
