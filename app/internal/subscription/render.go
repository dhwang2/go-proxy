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
	// Family is the address family this link's target belongs to, empty when
	// only one target was resolved.
	Family  string `json:"family,omitempty"`
	Content string `json:"content"`
}

type Renderer struct {
	// ambiguous marks a user and protocol pair that has more than one node, so
	// the link names for it need the port to stay distinct.
	ambiguous map[string]bool
	store     *store.Store
	nodes     map[string]*renderNode
	host      string
	targets   []SurgeTarget
}

type renderNode struct {
	inbound *store.Inbound
	users   map[string]*store.User
	binding *service.ShadowTLSBinding
	formats []Format
	tls     *clientTLS
}

func NewRenderer(s *store.Store, bindings []service.ShadowTLSBinding, host string, targets []SurgeTarget) *Renderer {
	r := &Renderer{store: s, host: host, targets: targets, nodes: make(map[string]*renderNode, len(s.SingBox.Inbounds)+1)}
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

func (r *Renderer) SetTargets(host string, targets []SurgeTarget) { r.host, r.targets = host, targets }

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
	links := make([]Link, 0, len(r.targets))
	for _, target := range r.targets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		family := ""
		if len(r.targets) > 1 {
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
				content = renderSurge(ib, entry, target.Host, r.host, name, u)
			}
		case FormatURI:
			content = renderURI(ib, entry, target.Host, name, u, node.tls)
		case FormatMihomo:
			content = renderMihomo(ib, entry, target.Host, name, u, node.tls)
		}
		if content == "" {
			return nil, fmt.Errorf("node %q has invalid or incomplete export credentials", entry.Tag)
		}
		links = append(links, Link{Proto: entry.Proto, Tag: entry.Tag, Port: port, UserName: entry.UserName, Family: target.Family, Content: content})
	}
	return links, nil
}

func shadowTLSBackendKey(proto string, port int) string { return proto + "|" + strconv.Itoa(port) }
