package application

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"

	"go-proxy/internal/cert"
	"go-proxy/internal/config"
	"go-proxy/internal/core"
	"go-proxy/internal/crypto"
	"go-proxy/internal/derived"
	"go-proxy/internal/protocol"
	"go-proxy/internal/routing"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
	"go-proxy/internal/user"
	"go-proxy/pkg/sysutil"
)

type ProtocolOptions struct {
	Type       protocol.Type
	User       string
	Port       string
	Domain     string
	Email      string
	SNI        string
	Congestion string
	// Snell server settings; empty means snell-server's default, or, when
	// joining the existing node, whatever it already has.
	Mode            string
	DNSIPPreference string
	DNS             string
	EgressInterface string
	ShadowTLS       bool
	ShadowTLSPort   string
	ShadowTLSSNI    string
}

type ProtocolNode struct {
	Tag             string           `json:"tag"`
	Type            string           `json:"type"`
	Port            int              `json:"port"`
	Transport       string           `json:"transport"`
	Security        string           `json:"security"`
	Users           []string         `json:"users"`
	SNI             string           `json:"sni,omitempty"`
	Congestion      string           `json:"congestion,omitempty"`
	Mode            string           `json:"mode,omitempty"`
	DNSIPPreference string           `json:"dns_ip_preference,omitempty"`
	ShadowTLS       *ProtocolWrapper `json:"shadow_tls,omitempty"`
	// Added names the user a `protocol add` enrolled, and AlreadyMember says
	// that user was enrolled before the command ran. Only an install sets
	// them.
	Added         string `json:"added,omitempty"`
	AlreadyMember bool   `json:"already_member,omitempty"`
}

type ProtocolWrapper struct {
	Service string `json:"service"`
	Port    int    `json:"port"`
	SNI     string `json:"sni"`
	Version int    `json:"version,omitempty"`
}

func protocolNodes(snapshot *Snapshot) []ProtocolNode {
	s := snapshot.Store
	nodes := make([]ProtocolNode, 0, len(s.SingBox.Inbounds)+1)
	for _, ib := range s.SingBox.Inbounds {
		n := ProtocolNode{Tag: ib.Tag, Type: ib.Type, Port: ib.ListenPort, Transport: "tcp", Security: "none", Users: []string{}, Congestion: ib.CongestionControl}
		if ib.Type == "tuic" {
			n.Transport = "udp"
		}
		if ib.TLS != nil {
			n.Security = "tls"
			n.SNI = ib.TLS.ServerName
		}
		if ib.HasReality() {
			n.Security = "reality"
		}
		for _, u := range ib.Users {
			n.Users = append(n.Users, u.Name)
		}
		nodes = append(nodes, n)
	}
	if s.SnellConf != nil {
		name := s.UserMeta.Name[store.UserKey("snell", store.SnellTag, s.SnellConf.PSK)]
		mode, preference := s.SnellConf.Settings()
		nodes = append(nodes, ProtocolNode{Tag: store.SnellTag, Type: "snell", Port: s.SnellConf.Port(), Transport: "tcp", Security: "none", Users: []string{name}, Mode: mode, DNSIPPreference: preference})
	}
	for i := range nodes {
		n := &nodes[i]
		sort.Strings(n.Users)
		for _, binding := range snapshot.Bindings {
			if binding.BackendProto == n.Type && binding.BackendPort == n.Port {
				n.ShadowTLS = &ProtocolWrapper{Service: binding.ServiceName, Port: binding.ListenPort, SNI: binding.SNI, Version: binding.Version}
			}
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Tag < nodes[j].Tag })
	return nodes
}

func (a *App) ProtocolList(ctx context.Context) (Result, error) {
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		return Result{}, err
	}
	return Result{Data: map[string]any{"nodes": protocolNodes(snapshot)}}, nil
}

func ValidateProtocolOptions(p ProtocolOptions) error {
	if !cert.IsValidEmail(p.Email) {
		return Invalid("invalid certificate email")
	}
	if err := user.ValidateName(p.User); err != nil {
		return Invalid(err.Error())
	}
	if _, err := parseProtocolPort(p.Port); err != nil {
		return err
	}
	spec, ok := protocol.Specs()[p.Type]
	if !ok || p.Type == protocol.ShadowTLS {
		return Invalid("unsupported protocol type")
	}
	// The domain is checked where it is needed rather than here: joining a node
	// that already exists inherits the domain that node was issued for, and
	// asking for it again is a question the answer to is already on disk.
	if p.Domain != "" && !cert.IsValidDomain(p.Domain) {
		return Invalid("--domain must be a valid domain")
	}
	if spec.UsesReality && p.Domain != "" {
		return Invalid("--domain cannot be combined with --reality")
	}
	if p.SNI != "" {
		if err := protocol.ValidateHandshakeDomain(p.SNI); err != nil {
			return Invalid(err.Error())
		}
	}
	if p.Type == protocol.TUIC && p.Congestion != "bbr" && p.Congestion != "cubic" {
		return Invalid("--congestion must be bbr or cubic")
	}
	if p.Type != protocol.Snell && (p.Mode != "" || p.DNSIPPreference != "" || p.DNS != "" || p.EgressInterface != "") {
		return Invalid("snell settings apply only to snell")
	}
	if err := validateSnellSettings(p); err != nil {
		return err
	}
	if p.ShadowTLS {
		if p.Type != protocol.Snell {
			return Invalid("shadow-tls requires snell")
		}
		if _, err := parseProtocolPort(p.ShadowTLSPort); err != nil {
			return Invalid("--shadow-tls-port must be a port or auto")
		}
		if p.ShadowTLSSNI != "" {
			if err := protocol.ValidateHandshakeDomain(p.ShadowTLSSNI); err != nil {
				return Invalid(err.Error())
			}
		}
	}
	return nil
}

// validateSnellSettings checks the snell-server keys against the values
// snell-server v6 documents. unsafe-raw is refused: it sends traffic in
// plaintext, which only an encrypting tunnel makes safe, and ShadowTLS
// authenticates what it carries without encrypting it.
func validateSnellSettings(p ProtocolOptions) error {
	if p.Mode == "unsafe-raw" {
		return Invalid("--mode unsafe-raw sends traffic in plaintext; use default or unshaped")
	}
	if p.Mode != "" && !slices.Contains(store.SnellModes, p.Mode) {
		return Invalid("--mode must be default or unshaped")
	}
	if p.DNSIPPreference != "" && !slices.Contains(store.SnellDNSIPPreferences, p.DNSIPPreference) {
		return Invalid("--dns-ip-preference must be one of " + strings.Join(store.SnellDNSIPPreferences, ", "))
	}
	if p.DNS != "" {
		for _, server := range strings.Split(p.DNS, ",") {
			if net.ParseIP(strings.TrimSpace(server)) == nil {
				return Invalid("--dns must be ip addresses separated by commas")
			}
		}
	}
	if p.EgressInterface != "" {
		if _, err := net.InterfaceByName(p.EgressInterface); err != nil {
			return Invalid("--egress-interface names no interface on this host")
		}
	}
	return nil
}

// snellConflicts reports whether a setting named in p differs from the
// existing node's. An unnamed setting keeps what the node has.
func snellConflicts(conf *store.SnellConfig, p ProtocolOptions) bool {
	if conf == nil {
		return false
	}
	differs := func(requested, current string) bool { return requested != "" && requested != current }
	mode, preference := conf.Settings()
	return differs(p.Mode, mode) || differs(p.DNSIPPreference, preference) ||
		differs(normalizeDNS(p.DNS), conf.DNS) || differs(p.EgressInterface, conf.EgressInterface)
}

// normalizeDNS writes a resolver list the way the config file keeps it.
func normalizeDNS(value string) string {
	if value == "" {
		return ""
	}
	servers := strings.Split(value, ",")
	for i := range servers {
		servers[i] = strings.TrimSpace(servers[i])
	}
	return strings.Join(servers, ",")
}

func parseProtocolPort(value string) (int, error) {
	if value == "auto" {
		return 0, nil
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, Invalid("port must be 1 to 65535 or auto")
	}
	return port, nil
}

// matchingNode finds the installed node an install request would join: the same
// protocol in the same form, on the requested port or on any port when the port
// is automatic. Shared with the check below so that what decides whether a
// domain is required is the same rule that decides which node is enrolled into.
func matchingNode(snapshot *Snapshot, p ProtocolOptions, port int) (*ProtocolNode, error) {
	var existing *ProtocolNode
	for _, node := range protocolNodes(snapshot) {
		wantType := protocol.Specs()[p.Type].DisplayName
		if p.Type == protocol.VLESSReality {
			wantType = "vless"
		}
		if p.Type == protocol.Snell {
			wantType = "snell"
		}
		matches := node.Type == wantType && (node.Security == "reality") == (p.Type == protocol.VLESSReality)
		if port != 0 && node.Port == port && !matches {
			return nil, Invalid("port belongs to a different protocol")
		}
		if matches && (port == 0 || node.Port == port) {
			if existing != nil {
				return nil, Invalid("multiple matching nodes; specify a numeric port")
			}
			n := node
			existing = &n
		}
	}
	return existing, nil
}

// needsCertificate reports whether creating this node requires a certificate,
// and therefore a domain to issue it for.
func needsCertificate(t protocol.Type) bool {
	spec := protocol.Specs()[t]
	return spec.NeedsTLS && !spec.UsesReality
}

func (a *App) ProtocolInstall(ctx context.Context, p ProtocolOptions) (Result, error) {
	if err := ValidateProtocolOptions(p); err != nil {
		return Result{}, err
	}
	// Enrolling into a node that already exists inherits the domain that node
	// was issued for, so --domain is required only when one has to be created.
	// Decided here, on a read-only snapshot that creates nothing, rather than
	// inside the operation: a missing argument must not follow a progress line
	// or leave operation state behind.
	if needsCertificate(p.Type) && !cert.IsValidDomain(p.Domain) {
		snapshot, err := a.Snapshot(ctx)
		if err != nil {
			return Result{}, err
		}
		port, _ := parseProtocolPort(p.Port)
		existing, err := matchingNode(snapshot, p, port)
		if err != nil {
			return Result{}, err
		}
		if existing == nil {
			return Result{}, Invalid("--domain is required to create a node; joining an existing one reuses its domain")
		}
	}
	// Read before the operation: a first install chooses the direct strategy
	// from the host's addresses, and a host with none leaves it unchosen.
	detected, _ := hostDirectStrategy(ctx)
	return a.Operation(ctx, func() (result Result, err error) {
		snapshot, err := a.Snapshot(ctx)
		if err != nil {
			return Result{}, err
		}
		s := snapshot.Store
		port, _ := parseProtocolPort(p.Port)
		existing, err := matchingNode(snapshot, p, port)
		if err != nil {
			return Result{}, err
		}
		if p.Type == protocol.Snell && s.SnellConf != nil && existing == nil {
			return Result{}, Invalid("snell already exists on another port")
		}
		if existing == nil && needsCertificate(p.Type) && !cert.IsValidDomain(p.Domain) {
			return Result{}, Invalid("--domain is required to create a node; joining an existing one reuses its domain")
		}
		if existing != nil {
			port = existing.Port
			if p.Type == protocol.TUIC && existing.Congestion != p.Congestion || p.Type == protocol.Snell && snellConflicts(s.SnellConf, p) {
				return Result{}, Invalid("existing node settings conflict with requested options")
			}
			if p.Domain != "" && existing.SNI != p.Domain || p.SNI != "" && existing.SNI != p.SNI {
				return Result{}, Invalid("existing node domain conflicts with requested domain")
			}
			// Inherited, so the certificate the node already uses is the one the
			// new member is enrolled against.
			if needsCertificate(p.Type) {
				p.Domain = existing.SNI
			}
			if p.Type == protocol.Snell && (len(existing.Users) != 1 || existing.Users[0] != p.User) {
				return Result{}, Invalid("snell supports only its existing owner")
			}
		}
		used := make(map[int]bool)
		for p := range derived.OccupiedPorts(s) {
			used[p] = true
		}
		for _, b := range snapshot.Bindings {
			used[b.ListenPort] = true
		}
		// Caddy's port is taken even while caddy-sub is stopped, when a
		// probe would find it free.
		if site, found := store.ReadCaddySite(); found {
			if existing == nil && port == site.Port {
				return Result{}, Invalid(fmt.Sprintf("port %d belongs to caddy; gproxy cert port <port> moves it", port))
			}
			used[site.Port] = true
		}
		if existing == nil {
			port, err = availableProtocolPort(p.Type, port, used)
			if err != nil {
				return Result{}, err
			}
		}
		used[port] = true
		var binding *service.ShadowTLSBinding
		// Snell is the only backend a shadow-tls listener fronts.
		const backend = "snell"
		for i := range snapshot.Bindings {
			b := &snapshot.Bindings[i]
			if b.BackendProto == backend && b.BackendPort == port {
				binding = b
			}
		}
		wrapperPort := 0
		if p.ShadowTLS {
			wrapperPort, _ = parseProtocolPort(p.ShadowTLSPort)
			if binding != nil {
				if wrapperPort != 0 && wrapperPort != binding.ListenPort || p.ShadowTLSSNI != "" && p.ShadowTLSSNI != binding.SNI {
					return Result{}, Invalid("existing shadow-tls binding conflicts with requested options")
				}
				wrapperPort = binding.ListenPort
			} else {
				wrapperPort, err = availableProtocolPort(protocol.ShadowTLS, wrapperPort, used)
				if err != nil {
					return Result{}, err
				}
			}
		}
		hasMember := false
		if existing != nil {
			for _, name := range existing.Users {
				if name == p.User {
					hasMember = true
				}
			}
		}
		changed := false
		stage := "prepare"
		data := map[string]any{"port": port, "user": p.User}
		defer func() {
			if err != nil && changed {
				var typed *Error
				if errors.As(err, &typed) {
					typed.Changed = true
				} else {
					err = &Error{Code: "protocol_install_failed", Message: err.Error(), Stage: stage, Changed: true, Data: data}
				}
			}
		}()
		if existing == nil && p.Type == protocol.VLESSReality {
			a.Progress("selecting and verifying reality handshake domain")
			p.SNI, err = protocol.SelectHandshakeDomain(ctx, p.SNI)
			if err != nil {
				return Result{}, err
			}
		}
		if p.ShadowTLS && binding == nil {
			a.Progress("selecting and verifying shadow-tls handshake domain")
			p.ShadowTLSSNI, err = protocol.SelectHandshakeDomain(ctx, p.ShadowTLSSNI)
			if err != nil {
				return Result{}, err
			}
		}
		component := core.CompSingBox
		svc := service.SingBox
		unit := config.SingBoxService
		provision := service.ProvisionSingBox
		if p.Type == protocol.Snell {
			component = core.CompSnell
			svc = service.Snell
			unit = config.SnellService
			provision = service.ProvisionSnell
		}
		// A step is reported only when it does work: the core is downloaded
		// the first time a protocol needs it, and then there is something to
		// wait for. An installed core is checked silently.
		if _, statErr := os.Stat(core.BinaryPath(component)); errors.Is(statErr, os.ErrNotExist) {
			changed = true
			a.Progress("downloading " + string(component))
		}
		if err = core.Ensure(ctx, component, ""); err != nil {
			return Result{}, err
		}
		if existing == nil && protocol.Specs()[p.Type].NeedsTLS && p.Type != protocol.VLESSReality {
			changed = true
			if err = cert.EnsureCertificateState(ctx, p.Domain, p.Email, a.Progress, func(fn func() error) error { return a.State(ctx, fn) }); err != nil {
				return Result{}, err
			}
		}
		if _, statErr := os.Stat(unit); errors.Is(statErr, os.ErrNotExist) {
			changed = true
			if err = a.State(ctx, func() error { return provision(ctx) }); err != nil {
				return Result{}, err
			}
		}
		if existing == nil {
			if _, err = user.Add(s, p.User, false); err != nil {
				return Result{}, err
			}
			installed, installErr := protocol.Install(s, protocol.InstallParams{ProtoType: p.Type, Port: port, UserName: p.User, Domain: p.Domain, SNI: p.SNI, CongestionControl: p.Congestion,
				SnellMode: p.Mode, SnellDNSIPPreference: p.DNSIPPreference, SnellDNS: normalizeDNS(p.DNS), SnellEgressInterface: p.EgressInterface})
			if installErr != nil {
				return Result{}, installErr
			}
			data["tag"] = installed.Tag
		} else {
			data["tag"] = existing.Tag
			if !hasMember {
				if _, err = protocol.AddUserToExisting(s, derived.FindInbound(s, existing.Tag), p.User); err != nil {
					return Result{}, err
				}
			}
		}
		strategyApplied := routing.ResolveDirectStrategy(s, detected)
		stage = "commit"
		configChanged := s.IsDirty()
		if err = a.Commit(ctx, snapshot); err != nil {
			return Result{}, err
		}
		changed = changed || configChanged
		services := []service.Name{svc}
		// A snell install that chose the strategy also rewrote sing-box's
		// configuration, which only a running sing-box has to pick up.
		if strategyApplied && svc != service.SingBox && len(s.SingBox.Inbounds) > 0 {
			services = append(services, service.SingBox)
		}
		if p.ShadowTLS && binding == nil {
			stage = "shadow_tls"
			if err = core.Ensure(ctx, core.CompShadowTLS, ""); err != nil {
				return Result{}, err
			}
			password, passwordErr := crypto.GeneratePassword(16)
			if passwordErr != nil {
				return Result{}, passwordErr
			}
			var name string
			changed = true
			if err = a.State(ctx, func() error {
				name, err = service.ProvisionShadowTLSBinding(ctx, backend, wrapperPort, password, p.ShadowTLSSNI, port)
				return err
			}); err != nil {
				return Result{}, err
			}
			snapshot.Bindings = append(snapshot.Bindings, service.ShadowTLSBinding{ServiceName: name, BackendProto: backend, BackendPort: port, ListenPort: wrapperPort, Password: password, SNI: p.ShadowTLSSNI, Version: 3})
			services = append(services, service.Name(name))
		} else if binding != nil {
			services = append(services, service.Name(binding.ServiceName))
		}
		stage = "activate"
		if err = a.Activate(ctx, snapshot, services...); err != nil {
			return Result{}, err
		}
		for _, node := range protocolNodes(snapshot) {
			if node.Port == port {
				node.Added, node.AlreadyMember = p.User, hasMember
				return Result{Changed: changed, Data: node}, nil
			}
		}
		return Result{Changed: changed, Data: data}, nil
	})
}

func availableProtocolPort(pt protocol.Type, port int, used map[int]bool) (int, error) {
	explicit := port != 0
	host := "0.0.0.0"
	if sysutil.IPv6Available() {
		host = "::"
	}
	for attempts := 0; attempts < 100; attempts++ {
		if !explicit {
			port = protocol.DefaultPort(pt, used)
		}
		if used[port] {
			if explicit {
				return 0, Invalid("port is already configured")
			}
			continue
		}
		var err error
		if pt != protocol.TUIC {
			var ln net.Listener
			ln, err = net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
			if err == nil {
				err = ln.Close()
			}
		}
		if err == nil && pt == protocol.TUIC {
			var conn net.PacketConn
			conn, err = net.ListenPacket("udp", net.JoinHostPort(host, strconv.Itoa(port)))
			if err == nil {
				err = conn.Close()
			}
		}
		if err == nil {
			return port, nil
		}
		if explicit {
			return 0, Invalid("port is unavailable")
		}
		used[port] = true
	}
	return 0, fmt.Errorf("no available protocol port found")
}

// NodeName is the name the removal guidance prints in its first column: the
// tag without the port, which is already the column beside it. Exported so the
// listing and the selector cannot disagree about what a node is called.
func NodeName(node ProtocolNode) string {
	return strings.TrimSuffix(node.Tag, "_"+strconv.Itoa(node.Port))
}

// selectNode resolves what the operator typed: the row number the removal
// guidance printed, the name it printed beside that number, or the full tag.
// The number is positional and the list is the one just shown, so it is read
// and used in the same breath; the tag stays accepted because that is what a
// script holds. The printed name is accepted because a command that prints a
// name and then refuses it is telling the reader something untrue -- two nodes
// of one protocol share a printed name, and that alone is ambiguous.
func selectNode(nodes []ProtocolNode, selector string) (*ProtocolNode, error) {
	for index := range nodes {
		if nodes[index].Tag == selector {
			return &nodes[index], nil
		}
	}
	if index, err := strconv.Atoi(selector); err == nil {
		if index < 1 || index > len(nodes) {
			return nil, Invalid(fmt.Sprintf("no node %d; there are %d", index, len(nodes)))
		}
		return &nodes[index-1], nil
	}
	var named *ProtocolNode
	ports := []string{}
	for index := range nodes {
		if NodeName(nodes[index]) != selector {
			continue
		}
		named = &nodes[index]
		ports = append(ports, strconv.Itoa(nodes[index].Port))
	}
	if len(ports) > 1 {
		return nil, Invalid(fmt.Sprintf("%s is on ports %s; select one by its row number or full tag", selector, strings.Join(ports, ", ")))
	}
	return named, nil
}

func (a *App) ProtocolRemove(ctx context.Context, tag, name string) (Result, error) {
	if tag == "" {
		return Result{}, Invalid("node tag is required")
	}
	if name != "" {
		if err := user.ValidateName(name); err != nil {
			return Result{}, Invalid(err.Error())
		}
	}
	return a.Operation(ctx, func() (Result, error) {
		snapshot, err := a.Snapshot(ctx)
		if err != nil {
			return Result{}, err
		}
		nodes := protocolNodes(snapshot)
		target, err := selectNode(nodes, tag)
		if err != nil {
			return Result{}, err
		}
		// removed states the outcome for the selector that was asked about, which
		// changed alone cannot: removing the last membership of a node and asking
		// to remove a node that is not there both leave the same envelope
		// otherwise, and the second one removed nothing.
		data := map[string]any{"tag": tag, "removed": false}
		if name != "" {
			data["user"] = name
		}
		if target == nil {
			return Result{Data: data}, nil
		}
		tag = target.Tag
		data["tag"] = tag
		if name != "" {
			// The node was found, so node_removed can answer for it from here
			// on: false while it stands, true once its last member takes it.
			data["node_removed"] = false
			found := false
			for _, n := range target.Users {
				if n == name {
					found = true
				}
			}
			if !found {
				// Nothing to remove. The answer still names the node the way
				// `user list` shows a membership, and says whether the user
				// exists at all, which is the difference between a typo and
				// asking the wrong node.
				data["membership"] = NodeMembership(target)
				data["user_exists"] = slices.Contains(derived.UserNames(snapshot.Store), name)
				return Result{Data: data}, nil
			}
		}
		// What each affected user had, taken before the removal changes it,
		// so the answer can show the membership that went, as `user list`
		// showed it.
		owners := target.Users
		if name != "" {
			owners = []string{name}
		}
		gone := make([]UserView, 0, len(owners))
		for _, owner := range owners {
			gone = append(gone, UserView{Name: owner, Memberships: []UserMembership{NodeMembership(target)}})
		}
		if name == "" {
			err = protocol.Remove(snapshot.Store, tag)
		} else {
			err = protocol.RemoveUserFromInbound(snapshot.Store, tag, name)
		}
		if err != nil {
			return Result{}, err
		}
		data["removed"] = true
		data["removed_memberships"] = gone
		if name != "" {
			// Removing the last member removes the node with it, so the result
			// has to say which happened: the node survived for its other users,
			// or it went. "node removed" alone said the second for both.
			gone := derived.FindInbound(snapshot.Store, tag) == nil
			if tag == store.SnellTag {
				gone = snapshot.Store.SnellConf == nil
			}
			data["node_removed"] = gone
		}
		svc := service.SingBox
		if tag == store.SnellTag {
			svc = service.Snell
		}
		return a.finishProtocolRemoval(ctx, snapshot, data, svc)
	})
}

// finishProtocolRemoval commits and activates whatever a removal left behind.
// data is the caller's own answer to what was removed: a node removal reports
// the node, a user deletion reports the user. Returning the remaining inventory
// for both told a caller deleting a user about nodes it had not asked about.
func (a *App) finishProtocolRemoval(ctx context.Context, snapshot *Snapshot, data map[string]any, services ...service.Name) (Result, error) {
	active := make(map[string]bool)
	for _, name := range derived.UserNames(snapshot.Store) {
		active[name] = true
	}
	if derived.PruneOrphanAuthUsers(snapshot.Store, active) {
		snapshot.Store.MarkDirty(store.FileUserRoutes)
		snapshot.Store.MarkDirty(store.FileSingBox)
		services = append(services, service.SingBox)
	}
	if err := a.Commit(ctx, snapshot); err != nil {
		return Result{}, err
	}
	kept := make([]service.ShadowTLSBinding, 0, len(snapshot.Bindings))
	for _, b := range snapshot.Bindings {
		exists := b.BackendProto == "snell" &&
			snapshot.Store.SnellConf != nil && snapshot.Store.SnellConf.Port() == b.BackendPort
		if exists {
			kept = append(kept, b)
			continue
		}
		if err := service.RemoveShadowTLSBindingByBackend(ctx, b.BackendProto, b.BackendPort); err != nil {
			return Result{}, &Error{Code: "binding_remove_failed", Message: err.Error(), Stage: "activate", Changed: true}
		}
	}
	snapshot.Bindings = kept
	if err := a.Activate(ctx, snapshot, services...); err != nil {
		return Result{}, err
	}
	return Result{Changed: true, Data: data}, nil
}

// NodeMembership is a node as `user list` shows one membership of it: the
// protocol under the name a membership carries (snell-v6 for the snell node),
// its port and any ShadowTLS wrapper.
func NodeMembership(node *ProtocolNode) UserMembership {
	name := node.Type
	if node.Tag == store.SnellTag {
		name = store.SnellTag
	}
	return UserMembership{Tag: node.Tag, Protocol: name, Port: node.Port, ShadowTLS: node.ShadowTLS}
}
