package derived

import "github.com/dhwang2/go-proxy/internal/store"

// CompileRoutes is the v2 route compilation entrypoint.
// Phase 1 keeps route rules as-is; full template compilation lands in Phase 3.
func CompileRoutes(config *store.SingboxConfig, _ *store.UserMeta) []store.RouteRule {
	if config == nil {
		return nil
	}
	out := make([]store.RouteRule, 0, len(config.Route.Rules))
	out = append(out, config.Route.Rules...)
	return out
}

func RouteRuleCount(config *store.SingboxConfig) int {
	if config == nil {
		return 0
	}
	return len(config.Route.Rules)
}
