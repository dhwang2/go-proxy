package application

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"

	"go-proxy/internal/cert"
	"go-proxy/internal/config"
	"go-proxy/internal/core"
	"go-proxy/internal/crypto"
	"go-proxy/internal/derived"
	"go-proxy/internal/protocol"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
	"go-proxy/internal/user"
	"go-proxy/pkg/sysutil"
)

type ProtocolOptions struct {
	Type          protocol.Type
	User          string
	Port          string
	Domain        string
	Email         string
	SNI           string
	Method        string
	Congestion    string
	IPv6          bool
	ShadowTLS     bool
	ShadowTLSPort string
	ShadowTLSSNI  string
}

type ProtocolNode struct {
	Tag        string           `json:"tag"`
	Type       string           `json:"type"`
	Port       int              `json:"port"`
	Transport  string           `json:"transport"`
	Security   string           `json:"security"`
	Users      []string         `json:"users"`
	SNI        string           `json:"sni,omitempty"`
	Method     string           `json:"method,omitempty"`
	Congestion string           `json:"congestion,omitempty"`
	IPv6       bool             `json:"ipv6,omitempty"`
	ShadowTLS  *ProtocolWrapper `json:"shadow_tls,omitempty"`
}

type ProtocolWrapper struct {
	Service string `json:"service"`
	Port    int    `json:"port"`
	SNI     string `json:"sni"`
}

func protocolNodes(snapshot *Snapshot) []ProtocolNode {
	s := snapshot.Store
	nodes := make([]ProtocolNode, 0, len(s.SingBox.Inbounds)+1)
	for _, ib := range s.SingBox.Inbounds {
		n := ProtocolNode{Tag: ib.Tag, Type: ib.Type, Port: ib.ListenPort, Transport: "tcp", Security: "none", Users: []string{}, Method: ib.Method, Congestion: ib.CongestionControl}
		if ib.Type == "shadowsocks" {
			n.Type = "ss"
			n.Transport = "tcp,udp"
		}
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
		nodes = append(nodes, ProtocolNode{Tag: store.SnellTag, Type: "snell", Port: s.SnellConf.Port(), Transport: "tcp", Security: "none", Users: []string{name}, IPv6: s.SnellConf.IPv6})
	}
	for i := range nodes {
		n := &nodes[i]
		sort.Strings(n.Users)
		for _, binding := range snapshot.Bindings {
			if binding.BackendProto == n.Type && binding.BackendPort == n.Port {
				n.ShadowTLS = &ProtocolWrapper{Service: binding.ServiceName, Port: binding.ListenPort, SNI: binding.SNI}
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
	if spec.NeedsTLS && !spec.UsesReality && !cert.IsValidDomain(p.Domain) {
		return Invalid("--domain is required and must be a valid domain")
	}
	if spec.UsesReality && p.Domain != "" {
		return Invalid("--domain cannot be combined with --reality")
	}
	if p.SNI != "" {
		if err := protocol.ValidateHandshakeDomain(p.SNI); err != nil {
			return Invalid(err.Error())
		}
	}
	if p.Type == protocol.Shadowsocks && p.Method != "2022-blake3-aes-128-gcm" && p.Method != crypto.DefaultSSMethod {
		return Invalid("unsupported shadowsocks method")
	}
	if p.Type == protocol.TUIC && p.Congestion != "bbr" && p.Congestion != "cubic" {
		return Invalid("--congestion must be bbr or cubic")
	}
	if p.ShadowTLS {
		if p.Type != protocol.Shadowsocks && p.Type != protocol.Snell {
			return Invalid("shadow-tls requires ss or snell")
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

func (a *App) ProtocolInstall(ctx context.Context, p ProtocolOptions) (Result, error) {
	if err := ValidateProtocolOptions(p); err != nil {
		return Result{}, err
	}
	return a.Operation(ctx, func() (result Result, err error) {
		snapshot, err := a.Snapshot(ctx)
		if err != nil {
			return Result{}, err
		}
		s := snapshot.Store
		port, _ := parseProtocolPort(p.Port)
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
				return Result{}, Invalid("port belongs to a different protocol")
			}
			if matches && (port == 0 || node.Port == port) {
				if existing != nil {
					return Result{}, Invalid("multiple matching nodes; specify a numeric port")
				}
				n := node
				existing = &n
			}
		}
		if p.Type == protocol.Snell && s.SnellConf != nil && existing == nil {
			return Result{}, Invalid("snell already exists on another port")
		}
		if existing != nil {
			port = existing.Port
			if p.Type == protocol.Shadowsocks && existing.Method != p.Method || p.Type == protocol.TUIC && existing.Congestion != p.Congestion || p.Type == protocol.Snell && existing.IPv6 != p.IPv6 {
				return Result{}, Invalid("existing node settings conflict with requested options")
			}
			if p.Domain != "" && existing.SNI != p.Domain || p.SNI != "" && existing.SNI != p.SNI {
				return Result{}, Invalid("existing node domain conflicts with requested domain")
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
		if existing == nil {
			port, err = availableProtocolPort(p.Type, port, used)
			if err != nil {
				return Result{}, err
			}
		}
		used[port] = true
		var binding *service.ShadowTLSBinding
		backend := "ss"
		if p.Type == protocol.Snell {
			backend = "snell"
		}
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
		if _, statErr := os.Stat(core.BinaryPath(component)); errors.Is(statErr, os.ErrNotExist) {
			changed = true
		}
		a.Progress("ensuring " + string(component) + " runtime")
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
			installed, installErr := protocol.Install(s, protocol.InstallParams{ProtoType: p.Type, Port: port, UserName: p.User, Domain: p.Domain, SNI: p.SNI, SSMethod: p.Method, CongestionControl: p.Congestion, SnellIPv6: p.IPv6})
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
		stage = "commit"
		configChanged := s.IsDirty()
		if err = a.Commit(ctx, snapshot); err != nil {
			return Result{}, err
		}
		changed = changed || configChanged
		services := []service.Name{svc}
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
		if err == nil && (pt == protocol.TUIC || pt == protocol.Shadowsocks) {
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
		var target *ProtocolNode
		for _, n := range protocolNodes(snapshot) {
			if n.Tag == tag {
				node := n
				target = &node
				break
			}
		}
		if target == nil {
			return Result{Data: map[string]any{"tag": tag}}, nil
		}
		if name != "" {
			found := false
			for _, n := range target.Users {
				if n == name {
					found = true
				}
			}
			if !found {
				return Result{Data: map[string]any{"tag": tag, "user": name}}, nil
			}
		}
		if name == "" {
			err = protocol.Remove(snapshot.Store, tag)
		} else {
			err = protocol.RemoveUserFromInbound(snapshot.Store, tag, name)
		}
		if err != nil {
			return Result{}, err
		}
		svc := service.SingBox
		if tag == store.SnellTag {
			svc = service.Snell
		}
		return a.finishProtocolRemoval(ctx, snapshot, svc)
	})
}

func (a *App) finishProtocolRemoval(ctx context.Context, snapshot *Snapshot, services ...service.Name) (Result, error) {
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
		exists := false
		for _, ib := range snapshot.Store.SingBox.Inbounds {
			if b.BackendProto == "ss" && ib.Type == "shadowsocks" && b.BackendPort == ib.ListenPort {
				exists = true
			}
		}
		if b.BackendProto == "snell" && snapshot.Store.SnellConf != nil && snapshot.Store.SnellConf.Port() == b.BackendPort {
			exists = true
		}
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
	return Result{Changed: true, Data: map[string]any{"nodes": protocolNodes(snapshot)}}, nil
}
