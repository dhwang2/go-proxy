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

	bindingsByBackend := shadowTLSBindingsByBackend(s)

	var links []Link
	for _, entry := range entries {
		var content string
		linkPort := entry.Port
		if entry.Tag == store.SnellTag {
			if s.SnellConf == nil {
				continue
			}
			binding := bindingsByBackend[shadowTLSBackendKey("snell", s.SnellConf.Port())]
			switch format {
			case FormatSurge:
				if binding != nil {
					content = renderShadowTLSSnellSurge(entry, s.SnellConf, *binding, host)
					linkPort = binding.ListenPort
				} else {
					content = renderSnellSurge(entry, s.SnellConf, host)
				}
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
				if binding != nil {
					content = renderShadowTLSShadowsocksSurge(ib, entry, *binding, host)
					linkPort = binding.ListenPort
				} else {
					content = renderSurge(ib, entry, host)
				}
			case FormatSingBox:
				if binding != nil {
					continue
				}
				content = renderSingBox(ib, entry, host)
			case FormatURI:
				content = renderURI(ib, entry, host)
			default:
				continue
			}
		}
		if content == "" {
			continue
		}
		links = append(links, Link{
			Proto:    entry.Proto,
			Tag:      entry.Tag,
			Port:     linkPort,
			UserName: entry.UserName,
			Content:  content,
		})
	}
	return links
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
