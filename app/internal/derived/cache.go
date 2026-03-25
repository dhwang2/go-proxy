package derived

import "go-proxy/internal/store"

// DashCache caches DashboardStats and recomputes only when the store version changes.
type DashCache struct {
	version  int64
	computed bool
	stats    DashboardStats
}

// Get returns cached DashboardStats, recomputing if the store has changed.
func (c *DashCache) Get(s *store.Store) DashboardStats {
	v := s.Version()
	if !c.computed || v != c.version {
		c.stats = Dashboard(s)
		c.version = v
		c.computed = true
	}
	return c.stats
}
