package subscription

import (
	"context"
	"fmt"
	"strconv"

	"go-proxy/internal/derived"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
)

type Format string

const (
	FormatSurge  Format = "surge"
	FormatURI    Format = "uri"
	FormatMihomo Format = "mihomo"
)

type Link struct {
	Proto    string `json:"protocol"`
	Tag      string `json:"tag"`
	Port     int    `json:"port"`
	UserName string `json:"user"`
	// Family is the address families this link's target serves: "dual",
	// "v4" or "v6", or empty when the export did not look them up.
	Family string `json:"family,omitempty"`
	// Name is the name the client shows, which a mihomo group lists.
	Name    string `json:"-"`
	Content string `json:"content"`
}

type Renderer struct {
	// ambiguous marks a user and protocol pair that has more than one node, so
	// the link names for it need the port to stay distinct.
	ambiguous map[string]bool
	store     *store.Store
	nodes     map[string]*renderNode
	targets   Targets
}

type renderNode struct {
	inbound *store.Inbound
	users   map[string]*store.User
	binding *service.ShadowTLSBinding
	formats []Format
	tls     *clientTLS
}

func NewRenderer(s *store.Store, bindings []service.ShadowTLSBinding, targets Targets) *Renderer {
	r := &Renderer{store: s, targets: targets, nodes: make(map[string]*renderNode, len(s.SingBox.Inbounds)+1)}
	byBackend := make(map[string]*service.ShadowTLSBinding, len(bindings))
	for i := range bindings {
		byBackend[shadowTLSBackendKey(bindings[i].BackendProto, bindings[i].BackendPort)] = &bindings[i]
	}
	for i := range s.SingBox.Inbounds {
		ib := &s.SingBox.Inbounds[i]
		n := &renderNode{inbound: ib}
		if ib.Type == "vless" || ib.Type == "tuic" {
			n.users = make(map[string]*store.User, len(ib.Users))
			for i := range ib.Users {
				n.users[ib.Users[i].Name] = &ib.Users[i]
			}
		}
		switch ib.Type {
		case "vless":
			n.formats = []Format{FormatURI, FormatMihomo}
		case "tuic", "anytls":
			n.formats = []Format{FormatURI, FormatSurge, FormatMihomo}
		}
		r.nodes[ib.Tag] = n
	}
	if s.SnellConf != nil {
		r.nodes[store.SnellTag] = &renderNode{binding: byBackend[shadowTLSBackendKey("snell", s.SnellConf.Port())], formats: []Format{FormatSurge}}
	}
	counts := map[string]int{}
	for name, entries := range derived.Membership(s) {
		for _, entry := range entries {
			counts[name+"\x00"+r.protoLabel(entry)]++
		}
	}
	r.ambiguous = make(map[string]bool, len(counts))
	for key, count := range counts {
		if count > 1 {
			r.ambiguous[key] = true
		}
	}
	return r
}

// protoLabel is the protocol as a link names it: the inbound's own type, or
// "snell" for the one node that is not an inbound.
func (r *Renderer) protoLabel(entry derived.MembershipEntry) string {
	if node := r.nodes[entry.Tag]; node != nil && node.inbound != nil {
		return node.inbound.Type
	}
	return "snell"
}

// linkName is the one place a link's name is built, so every format calls the
// same node the same thing.
func (r *Renderer) linkName(entry derived.MembershipEntry, family string) string {
	label := r.protoLabel(entry)
	return proxyName(label, entry.UserName, family, r.ambiguous[entry.UserName+"\x00"+label], entry.Port)
}

func (r *Renderer) SetTargets(targets Targets) { r.targets = targets }

func (r *Renderer) Formats(entry derived.MembershipEntry) []Format {
	if node := r.nodes[entry.Tag]; node != nil {
		return node.formats
	}
	return nil
}

func (r *Renderer) Render(ctx context.Context, entry derived.MembershipEntry, format Format) ([]Link, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	allowed := false
	for _, f := range r.Formats(entry) {
		if format == f {
			allowed = true
		}
	}
	if !allowed {
		return nil, fmt.Errorf("node %q does not support %s export", entry.Tag, format)
	}
	if entry.UserID == "" {
		return nil, fmt.Errorf("node %q has missing credentials", entry.Tag)
	}
	node := r.nodes[entry.Tag]
	ib, binding := node.inbound, node.binding
	u := node.users[entry.UserName]
	if ib != nil && ib.Type == "tuic" {
		if u == nil || u.Password == "" {
			return nil, fmt.Errorf("node %q has missing tuic credentials", entry.Tag)
		}
	}
	if ib != nil && ib.TLS != nil && node.tls == nil && (format == FormatMihomo ||
		format == FormatURI && ib.Type == "vless") {
		var err error
		node.tls, err = buildClientTLS(ib.TLS)
		if err != nil {
			return nil, fmt.Errorf("node %q has invalid tls configuration: %w", entry.Tag, err)
		}
	}
	port := entry.Port
	if binding != nil {
		if binding.Password == "" || binding.SNI == "" {
			return nil, fmt.Errorf("node %q has incomplete shadow-tls configuration", entry.Tag)
		}
		port = binding.ListenPort
	}
	targets := r.targets.Links
	if format == FormatMihomo {
		targets = r.targets.Mihomo
	}
	links := make([]Link, 0, len(targets))
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// A family in the name only where a node has a link per family.
		family := ""
		if len(targets) > 1 {
			family = target.Family
		}
		name := r.linkName(entry, family)
		var content string
		switch format {
		case FormatSurge:
			if entry.Tag == store.SnellTag {
				if binding != nil {
					content = renderShadowTLSSnellSurge(entry, r.store.SnellConf, *binding, target.Host, name)
				} else {
					content = renderSnellSurge(entry, r.store.SnellConf, target.Host, name)
				}
			} else {
				content = renderSurge(ib, entry, target.Host, r.targets.Host, name, u)
			}
			content = withSurgeIPVersion(content, target)
		case FormatURI:
			content = renderURI(ib, entry, target.Host, name, u, node.tls)
		case FormatMihomo:
			content = renderMihomo(ib, entry, target, name, u, node.tls)
		}
		if content == "" {
			return nil, fmt.Errorf("node %q has invalid or incomplete export credentials", entry.Tag)
		}
		links = append(links, Link{Proto: entry.Proto, Tag: entry.Tag, Port: port, UserName: entry.UserName, Family: target.Family, Name: name, Content: content})
	}
	return links, nil
}

// MihomoGroup joins a node's per-family mihomo entries into one fallback
// group under the node's own name, IPv4 first, so a client picks the node once
// and moves to IPv6 when IPv4 fails its health check. A mihomo proxy has one
// server, so an address-based node on a dual-stack server needs an entry per
// family. "" when the node has one entry.
func (r *Renderer) MihomoGroup(entry derived.MembershipEntry, links []Link) string {
	if len(links) < 2 {
		return ""
	}
	members := make([]string, 0, len(links))
	for _, link := range links {
		members = append(members, link.Name)
	}
	return flow([]field{
		{"name", r.linkName(entry, "")},
		{"type", "fallback"},
		{"proxies", members},
		{"url", "http://www.gstatic.com/generate_204"},
		{"interval", 300},
		{"lazy", true},
	})
}

func shadowTLSBackendKey(proto string, port int) string { return proto + "|" + strconv.Itoa(port) }
