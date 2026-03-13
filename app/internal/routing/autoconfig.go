package routing

import (
	"strings"

	"github.com/dhwang2/go-proxy/internal/store"
)

// Reconcile ensures routing and DNS rules are consistent with the current
// config state. Cleans up orphaned rules referencing deleted users or outbounds.
func Reconcile(st *store.Store) (MutationResult, error) {
	if st == nil || st.Config == nil || st.UserMeta == nil {
		return MutationResult{}, nil
	}

	// Build sets of valid users and outbounds.
	validUsers := make(map[string]struct{})
	for name := range st.UserMeta.Groups {
		validUsers[normalizeName(name)] = struct{}{}
	}
	// Also include users from inbounds.
	for _, inb := range st.Config.Inbounds {
		for _, u := range inb.Users {
			if n := normalizeName(u.Name); n != "" {
				validUsers[n] = struct{}{}
			}
		}
	}

	validOutbounds := make(map[string]struct{})
	for _, ob := range st.Config.Outbounds {
		tag := strings.TrimSpace(ob.Tag)
		if tag != "" {
			validOutbounds[tag] = struct{}{}
		}
	}
	// "direct" and "block" are always valid built-in outbounds.
	validOutbounds["direct"] = struct{}{}
	validOutbounds["block"] = struct{}{}

	// Walk route rules and remove orphaned ones.
	next := make([]store.RouteRule, 0, len(st.Config.Route.Rules))
	removed := 0
	for _, rule := range st.Config.Route.Rules {
		action := strings.TrimSpace(rule.Action)
		if action != "route" || len(rule.RuleSet) == 0 {
			next = append(next, rule)
			continue
		}

		// Check if outbound exists.
		outbound := strings.TrimSpace(rule.Outbound)
		if _, ok := validOutbounds[outbound]; !ok && outbound != "" {
			removed++
			continue
		}

		// Filter auth_users to only valid ones.
		if len(rule.AuthUser) > 0 {
			filtered := make([]string, 0, len(rule.AuthUser))
			for _, u := range rule.AuthUser {
				if _, ok := validUsers[normalizeName(u)]; ok {
					filtered = append(filtered, u)
				}
			}
			if len(filtered) == 0 {
				removed++
				continue
			}
			rule.AuthUser = filtered
		}

		next = append(next, rule)
	}

	if removed == 0 {
		return MutationResult{}, nil
	}

	st.Config.Route.Rules = next
	dnsChanged := rebuildManagedDNS(st)
	st.MarkConfigDirty()
	return MutationResult{
		ConfigChanged: true,
		RouteChanged:  removed,
		DNSChanged:    dnsChanged,
	}, nil
}
