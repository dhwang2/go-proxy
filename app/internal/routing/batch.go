package routing

import (
	"fmt"
	"sort"

	"go-proxy/internal/store"
)

func SetRules(s *store.Store, userName string, rules []store.UserRouteRule) (int, error) {
	count := 0
	for _, rule := range rules {
		if err := SetRule(s, userName, rule); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func DeleteUserRulesByIndex(s *store.Store, userName string, indexes []int) (int, error) {
	if userName == "" {
		return 0, fmt.Errorf("user name cannot be empty")
	}
	selected := normalizeRuleIndexes(indexes)
	if len(selected) == 0 {
		return 0, nil
	}

	position := 0
	removed := 0
	kept := make([]store.UserRouteRule, 0, len(s.UserRoutes))
	for _, rule := range s.UserRoutes {
		if !hasAuthUser(rule.AuthUser, userName) {
			kept = append(kept, rule)
			continue
		}
		position++
		if !selected[position] {
			kept = append(kept, rule)
			continue
		}
		var users []string
		for _, name := range rule.AuthUser {
			if name != userName {
				users = append(users, name)
			}
		}
		if len(users) > 0 {
			rule.AuthUser = users
			kept = append(kept, rule)
		}
		removed++
	}
	if removed > 0 {
		s.UserRoutes = kept
		s.MarkDirty(store.FileUserRoutes)
	}
	return removed, nil
}

func ReplaceUserRuleOutbounds(s *store.Store, userName string, indexes []int, outbound string) (int, error) {
	if userName == "" {
		return 0, fmt.Errorf("user name cannot be empty")
	}
	if outbound == "" {
		return 0, fmt.Errorf("outbound cannot be empty")
	}
	selected := normalizeRuleIndexes(indexes)
	if len(selected) == 0 {
		return 0, nil
	}

	position := 0
	updated := 0
	rebuilt := make([]store.UserRouteRule, 0, len(s.UserRoutes)+len(selected))
	for _, rule := range s.UserRoutes {
		if !hasAuthUser(rule.AuthUser, userName) {
			rebuilt = append(rebuilt, rule)
			continue
		}
		position++
		if !selected[position] {
			rebuilt = append(rebuilt, rule)
			continue
		}
		if len(rule.AuthUser) > 1 {
			cloned := rule
			rule.AuthUser = removeAuthUser(rule.AuthUser, userName)
			cloned.AuthUser = []string{userName}
			cloned.Outbound = outbound
			if cloned.Action == "" {
				cloned.Action = "route"
			}
			rebuilt = append(rebuilt, rule, cloned)
		} else {
			rule.Outbound = outbound
			if rule.Action == "" {
				rule.Action = "route"
			}
			rebuilt = append(rebuilt, rule)
		}
		updated++
	}
	if updated > 0 {
		s.UserRoutes = rebuilt
		s.MarkDirty(store.FileUserRoutes)
	}
	return updated, nil
}

func normalizeRuleIndexes(indexes []int) map[int]bool {
	selected := make(map[int]bool, len(indexes))
	for _, idx := range indexes {
		if idx > 0 {
			selected[idx] = true
		}
	}
	return selected
}

func ExpandRuleIndexes(total int, indexes []int, all bool) []int {
	if !all {
		out := make([]int, 0, len(indexes))
		seen := normalizeRuleIndexes(indexes)
		for idx := range seen {
			if idx <= total {
				out = append(out, idx)
			}
		}
		sort.Ints(out)
		return out
	}
	out := make([]int, 0, total)
	for i := 1; i <= total; i++ {
		out = append(out, i)
	}
	return out
}
