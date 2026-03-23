package derived

import "go-proxy/internal/store"

// ProtocolInfo describes an installed protocol.
type ProtocolInfo struct {
	Type       string // inbound type (vless, tuic, trojan, shadowsocks, anytls)
	Tag        string // inbound tag
	Port       int    // listen port
	UserCount  int    // number of users
	HasReality bool   // uses Reality TLS
}

// Inventory returns the list of installed protocols from inbounds.
func Inventory(s *store.Store) []ProtocolInfo {
	var result []ProtocolInfo
	for _, ib := range s.SingBox.Inbounds {
		info := ProtocolInfo{
			Type:      ib.Type,
			Tag:       ib.Tag,
			Port:      ib.ListenPort,
			UserCount: len(ib.Users),
		}
		info.HasReality = ib.HasReality()
		// Shadowsocks single-user: count as 1.
		if ib.Type == "shadowsocks" && len(ib.Users) == 0 && ib.Password != "" {
			info.UserCount = 1
		}
		result = append(result, info)
	}
	if s.SnellConf != nil {
		if port := s.SnellConf.Port(); port > 0 {
			result = append(result, ProtocolInfo{
				Type:      store.SnellTag,
				Tag:       store.SnellTag,
				Port:      port,
				UserCount: 1,
			})
		}
	}
	return result
}

// OccupiedPorts returns all ports currently used by inbounds.
func OccupiedPorts(s *store.Store) map[int]string {
	ports := make(map[int]string)
	for _, ib := range s.SingBox.Inbounds {
		if ib.ListenPort > 0 {
			ports[ib.ListenPort] = ib.Tag
		}
	}
	if s.SnellConf != nil {
		if port := s.SnellConf.Port(); port > 0 {
			ports[port] = store.SnellTag
		}
	}
	return ports
}

// FindInbound returns the inbound with the given tag, or nil.
func FindInbound(s *store.Store, tag string) *store.Inbound {
	for i := range s.SingBox.Inbounds {
		if s.SingBox.Inbounds[i].Tag == tag {
			return &s.SingBox.Inbounds[i]
		}
	}
	return nil
}
