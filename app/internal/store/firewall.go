package store

// FirewallConfig stores additional custom ports that should remain open.
type FirewallConfig struct {
	TCP []int `json:"tcp,omitempty"`
	UDP []int `json:"udp,omitempty"`
}

func (c *FirewallConfig) Normalize() {
	if c == nil {
		return
	}
	c.TCP = normalizePorts(c.TCP)
	c.UDP = normalizePorts(c.UDP)
}

func normalizePorts(ports []int) []int {
	if len(ports) == 0 {
		return nil
	}
	seen := make(map[int]bool, len(ports))
	out := make([]int, 0, len(ports))
	for _, port := range ports {
		if port <= 0 || port > 65535 || seen[port] {
			continue
		}
		seen[port] = true
		out = append(out, port)
	}
	return out
}
