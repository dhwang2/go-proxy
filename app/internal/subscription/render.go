package subscription

import (
	"strconv"

	"go-proxy/internal/derived"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
)

var listShadowTLSBindings = service.ListShadowTLSBindings

// Format represents a subscription output format.
type Format string

const (
	FormatSurge   Format = "surge"
	FormatSingBox Format = "singbox"
	FormatURI     Format = "uri"
)

// Link holds a generated subscription link or config for one protocol membership.
type Link struct {
	Proto    string
	Tag      string
	Port     int
	UserName string
	Content  string // the generated link or config snippet
}

// Render generates subscription links for a user in the specified format.
func Render(s *store.Store, userName string, format Format, host string) []Link {
	membership := derived.Membership(s)
	entries, ok := membership[userName]
	if !ok {
		return nil
	}

	if host == "" {
		host = DetectTarget()
	}

	// For Surge format, detect dual-stack targets once and reuse across entries.
	var surgeTargets []SurgeTarget
	if format == FormatSurge {
		surgeTargets = DetectSurgeTargets(host)
	}

	bindingsByBackend := shadowTLSBindingsByBackend(s)

	var links []Link
	for _, entry := range entries {
		linkPort := entry.Port
		if entry.Tag == store.SnellTag {
			if s.SnellConf == nil {
				continue
			}
			binding := bindingsByBackend[shadowTLSBackendKey("snell", s.SnellConf.Port())]
			switch format {
			case FormatSurge:
				port := linkPort
				if binding != nil {
					port = binding.ListenPort
				}
				for _, t := range surgeTargets {
					suffix := surgeTagSuffix(t, surgeTargets)
					var content string
					if binding != nil {
						content = renderShadowTLSSnellSurge(entry, s.SnellConf, *binding, t.Host, suffix)
					} else {
						content = renderSnellSurge(entry, s.SnellConf, t.Host, suffix)
					}
					links = appendLink(links, entry, port, content)
				}
				continue
			default:
				continue
			}
		} else {
			ib := derived.FindInbound(s, entry.Tag)
			if ib == nil {
				continue
			}
			var binding *service.ShadowTLSBinding
			if ib.Type == "shadowsocks" {
				binding = bindingsByBackend[shadowTLSBackendKey("ss", ib.ListenPort)]
			}
			switch format {
			case FormatSurge:
				port := linkPort
				if binding != nil {
					port = binding.ListenPort
				}
				for _, t := range surgeTargets {
					suffix := surgeTagSuffix(t, surgeTargets)
					var content string
					if binding != nil {
						content = renderShadowTLSShadowsocksSurge(ib, entry, *binding, t.Host, suffix)
					} else {
						content = renderSurge(ib, entry, t.Host, host, suffix)
					}
					links = appendLink(links, entry, port, content)
				}
				continue
			case FormatSingBox:
				if binding != nil {
					continue
				}
				links = appendLink(links, entry, linkPort, renderSingBox(ib, entry, host))
				continue
			case FormatURI:
				links = appendLink(links, entry, linkPort, renderURI(ib, entry, host))
				continue
			default:
				continue
			}
		}
	}
	return links
}

// surgeTagSuffix returns "-v4" or "-v6" when there are multiple targets, empty string for single-stack.
func surgeTagSuffix(t SurgeTarget, all []SurgeTarget) string {
	if len(all) <= 1 || t.Family == "" {
		return ""
	}
	return "-" + t.Family
}

func appendLink(links []Link, entry derived.MembershipEntry, port int, content string) []Link {
	if content == "" {
		return links
	}
	return append(links, Link{
		Proto:    entry.Proto,
		Tag:      entry.Tag,
		Port:     port,
		UserName: entry.UserName,
		Content:  content,
	})
}

func shadowTLSBindingsByBackend(s *store.Store) map[string]*service.ShadowTLSBinding {
	bindings, err := listShadowTLSBindings(s)
	if err != nil {
		return nil
	}
	result := make(map[string]*service.ShadowTLSBinding, len(bindings))
	for i := range bindings {
		result[shadowTLSBackendKey(bindings[i].BackendProto, bindings[i].BackendPort)] = &bindings[i]
	}
	return result
}

func shadowTLSBackendKey(proto string, port int) string {
	return proto + "|" + strconv.Itoa(port)
}
