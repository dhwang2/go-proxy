package protocol

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dhwang2/go-proxy/internal/crypto"
	"github.com/dhwang2/go-proxy/internal/store"
)

type InstallSpec struct {
	Protocol string
	Tag      string
	Port     int
	Group    string
	UserID   string
	Secret   string
	Method   string
	SNI      string
}

type MutationResult struct {
	ConfigChanged bool
	MetaChanged   bool
	SnellChanged  bool

	AddedInbounds   int
	RemovedInbounds int
	UpdatedMetaRows int
}

func (r MutationResult) Changed() bool {
	return r.ConfigChanged || r.MetaChanged || r.SnellChanged
}

func Install(st *store.Store, spec InstallSpec) (MutationResult, error) {
	if st == nil || st.Config == nil || st.UserMeta == nil {
		return MutationResult{}, fmt.Errorf("store is nil")
	}
	proto, err := normalizeProtocol(spec.Protocol)
	if err != nil {
		return MutationResult{}, err
	}
	if proto == "snell" {
		return installSnell(st, spec)
	}

	port, err := resolveInstallPort(st, proto, spec.Port)
	if err != nil {
		return MutationResult{}, err
	}
	tag := strings.TrimSpace(spec.Tag)
	if tag == "" {
		tag = fmt.Sprintf("%s-%d", proto, port)
	}
	if inboundTagExists(st.Config, tag) {
		return MutationResult{}, fmt.Errorf("inbound tag already exists: %s", tag)
	}

	group := normalizeGroupName(spec.Group)
	if group == "" {
		group = defaultGroupName(st.UserMeta)
	}
	userID, secret, method, flow, err := resolveUserFields(proto, spec)
	if err != nil {
		return MutationResult{}, err
	}

	inbound := store.Inbound{
		Type:       protocolTypeForConfig(proto),
		Tag:        tag,
		ListenPort: port,
		Raw:        map[string]json.RawMessage{},
	}
	// Listen on all interfaces by default.
	writeRawString(inbound.Raw, "listen", "::")
	user := store.User{
		Name:     group,
		UUID:     userUUIDForProtocol(proto, userID),
		ID:       userID,
		Password: secret,
	}
	// Set method at inbound level for SS (sing-box rejects per-user method).
	if method != "" {
		writeRawString(inbound.Raw, "method", method)
	}
	// For SS 2022-blake3 methods, set server PSK at inbound level.
	if proto == "ss" && crypto.IsSSAEAD2022(method) {
		serverPSK, pskErr := crypto.NewSSKey(method)
		if pskErr != nil {
			return MutationResult{}, fmt.Errorf("generate ss server key: %w", pskErr)
		}
		writeRawString(inbound.Raw, "password", serverPSK)
	}
	if flow != "" {
		user.Raw = map[string]json.RawMessage{}
		writeRawString(user.Raw, "flow", flow)
	}
	inbound.Users = []store.User{user}

	if protocolNeedsTLS(proto) {
		tlsRaw, err := resolveTLSRaw(st.Config, proto, spec.SNI)
		if err != nil {
			return MutationResult{}, err
		}
		if len(tlsRaw) > 0 {
			// Ensure enabled:true is set on TLS config.
			var tlsMap map[string]json.RawMessage
			if json.Unmarshal(tlsRaw, &tlsMap) == nil {
				writeRawBool(tlsMap, "enabled", true)
				if b, err := json.Marshal(tlsMap); err == nil {
					tlsRaw = b
				}
			}
			inbound.Raw["tls"] = tlsRaw
		}
	}
	st.Config.Inbounds = append(st.Config.Inbounds, inbound)

	result := MutationResult{ConfigChanged: true, AddedInbounds: 1}
	if ensureMetaForUser(st.UserMeta, proto, tag, userID, group) {
		result.MetaChanged = true
		result.UpdatedMetaRows++
	}

	if result.ConfigChanged {
		st.MarkConfigDirty()
	}
	if result.MetaChanged {
		st.MarkUserMetaDirty()
	}
	return result, nil
}

func installSnell(st *store.Store, spec InstallSpec) (MutationResult, error) {
	if st.SnellConf == nil {
		st.SnellConf = &store.SnellConfig{Values: map[string]string{}}
	}
	port, err := resolveInstallPort(st, "snell", spec.Port)
	if err != nil {
		return MutationResult{}, err
	}
	secret := strings.TrimSpace(spec.Secret)
	if secret == "" {
		secret, err = crypto.NewPassword(24)
		if err != nil {
			return MutationResult{}, err
		}
	}
	group := normalizeGroupName(spec.Group)
	if group == "" {
		group = defaultGroupName(st.UserMeta)
	}

	result := MutationResult{}
	listenAddr := "0.0.0.0"
	if strings.EqualFold(strings.TrimSpace(st.SnellConf.Values["ipv6"]), "true") {
		listenAddr = "::"
	}
	listen := listenAddr + ":" + strconv.Itoa(port)
	if strings.TrimSpace(st.SnellConf.Values["listen"]) != listen {
		st.SnellConf.Values["listen"] = listen
		result.SnellChanged = true
	}
	if strings.TrimSpace(st.SnellConf.Values["psk"]) != secret {
		st.SnellConf.Values["psk"] = secret
		result.SnellChanged = true
	}
	// Ensure defaults for optional snell fields.
	for _, kv := range []struct{ key, def string }{
		{"ipv6", "false"},
		{"udp", "true"},
		{"obfs", "off"},
	} {
		if strings.TrimSpace(st.SnellConf.Values[kv.key]) == "" {
			st.SnellConf.Values[kv.key] = kv.def
			result.SnellChanged = true
		}
	}
	if ensureMetaForUser(st.UserMeta, "snell", "snell-v5", secret, group) {
		result.MetaChanged = true
		result.UpdatedMetaRows++
	}
	if result.SnellChanged {
		st.MarkSnellDirty()
	}
	if result.MetaChanged {
		st.MarkUserMetaDirty()
	}
	return result, nil
}

func resolveUserFields(proto string, spec InstallSpec) (userID, secret, method, flow string, err error) {
	userID = strings.TrimSpace(spec.UserID)
	secret = strings.TrimSpace(spec.Secret)
	method = strings.TrimSpace(spec.Method)

	switch proto {
	case "vless":
		if userID == "" {
			userID, err = crypto.NewUUIDv4()
			if err != nil {
				return "", "", "", "", err
			}
		}
		flow = "xtls-rprx-vision"
	case "tuic":
		if userID == "" {
			userID, err = crypto.NewUUIDv4()
			if err != nil {
				return "", "", "", "", err
			}
		}
		if secret == "" {
			secret, err = crypto.NewPassword(24)
			if err != nil {
				return "", "", "", "", err
			}
		}
	case "trojan", "anytls":
		if secret == "" {
			secret, err = crypto.NewPassword(24)
			if err != nil {
				return "", "", "", "", err
			}
		}
		userID = secret
	case "ss":
		if method == "" {
			method = "2022-blake3-aes-128-gcm"
		}
		if secret == "" {
			if crypto.IsSSAEAD2022(method) {
				secret, err = crypto.NewSSKey(method)
			} else {
				secret, err = crypto.NewPassword(24)
			}
			if err != nil {
				return "", "", "", "", err
			}
		}
		userID = secret
	default:
		return "", "", "", "", fmt.Errorf("unsupported protocol: %s", proto)
	}
	return userID, secret, method, flow, nil
}

func writeRawString(raw map[string]json.RawMessage, key, value string) {
	if raw == nil || key == "" || strings.TrimSpace(value) == "" {
		return
	}
	b, _ := json.Marshal(value)
	raw[key] = b
}

func writeRawBool(raw map[string]json.RawMessage, key string, value bool) {
	if raw == nil || key == "" {
		return
	}
	b, _ := json.Marshal(value)
	raw[key] = b
}

func userUUIDForProtocol(proto, userID string) string {
	if proto == "vless" || proto == "tuic" {
		return userID
	}
	return ""
}

func ensureMetaForUser(meta *store.UserMeta, proto, tag, userID, group string) bool {
	if meta == nil {
		return false
	}
	meta.EnsureDefaults()
	group = normalizeGroupName(group)
	if group == "" {
		group = "user"
	}
	meta.AddGroup(group)

	key := userMetaKey(proto, tag, userID)
	if strings.TrimSpace(meta.Name[key]) == group {
		return false
	}
	meta.Name[key] = group
	return true
}

func resolveInstallPort(st *store.Store, proto string, requested int) (int, error) {
	used := usedPorts(st)
	if requested > 0 {
		if used[requested] {
			return 0, fmt.Errorf("port %d already in use by config", requested)
		}
		if !PortAvailable(requested) {
			return 0, fmt.Errorf("port %d is not available on host", requested)
		}
		return requested, nil
	}
	start := defaultPortForProtocol(proto)
	for p := start; p < start+200; p++ {
		if used[p] {
			continue
		}
		if !PortAvailable(p) {
			continue
		}
		return p, nil
	}
	return 0, fmt.Errorf("no available port found for %s", proto)
}

func inboundTagExists(cfg *store.SingboxConfig, tag string) bool {
	if cfg == nil {
		return false
	}
	tag = strings.TrimSpace(tag)
	for _, in := range cfg.Inbounds {
		if strings.TrimSpace(in.Tag) == tag {
			return true
		}
	}
	return false
}

// DefaultPort returns the default starting port for a protocol.
func DefaultPort(proto string) int {
	return defaultPortForProtocol(proto)
}

func defaultPortForProtocol(proto string) int {
	switch proto {
	case "vless":
		return 443
	case "trojan":
		return 8443
	case "tuic":
		return 4433
	case "anytls":
		return 10443
	case "ss":
		return 8388
	case "snell":
		return 8444
	default:
		return 20000
	}
}

// UsedPorts returns a set of ports currently occupied by configured protocols.
func UsedPorts(st *store.Store) map[int]bool {
	return usedPorts(st)
}

func usedPorts(st *store.Store) map[int]bool {
	used := map[int]bool{}
	if st != nil && st.Config != nil {
		for _, in := range st.Config.Inbounds {
			if in.ListenPort > 0 {
				used[in.ListenPort] = true
			}
		}
	}
	if st != nil && st.SnellConf != nil {
		listen := strings.TrimSpace(st.SnellConf.Get("listen"))
		if p := parseListenPort(listen); p > 0 {
			used[p] = true
		}
	}
	return used
}

func parseListenPort(listen string) int {
	listen = strings.TrimSpace(listen)
	if listen == "" {
		return 0
	}
	parts := strings.Split(listen, ":")
	v := strings.TrimSpace(parts[len(parts)-1])
	p, _ := strconv.Atoi(v)
	if p > 0 {
		return p
	}
	return 0
}

func normalizeProtocol(raw string) (string, error) {
	s := normalizeProtocolType(raw)
	switch s {
	case "vless", "trojan", "tuic", "anytls", "ss", "snell":
		return s, nil
	default:
		return "", fmt.Errorf("unsupported protocol %q (supported: vless|trojan|tuic|anytls|ss|snell)", raw)
	}
}

func normalizeProtocolType(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case "shadowsocks", "ss":
		return "ss"
	default:
		return s
	}
}

func protocolTypeForConfig(proto string) string {
	if proto == "ss" {
		return "shadowsocks"
	}
	return proto
}

func defaultGroupName(meta *store.UserMeta) string {
	if meta != nil {
		names := make([]string, 0, len(meta.Groups))
		for name := range meta.Groups {
			n := normalizeGroupName(name)
			if n != "" {
				names = append(names, n)
			}
		}
		sort.Strings(names)
		if len(names) > 0 {
			return names[0]
		}
	}
	return "user"
}

func normalizeGroupName(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return ""
	}
	v = strings.NewReplacer(" ", "-", "\t", "-", "\n", "-").Replace(v)
	for strings.Contains(v, "--") {
		v = strings.ReplaceAll(v, "--", "-")
	}
	return strings.Trim(v, "-_.")
}

func userMetaKey(proto, tag, userID string) string {
	return normalizeProtocolType(proto) + "|" + strings.TrimSpace(tag) + "|" + strings.TrimSpace(userID)
}

// NeedsTLS returns true if the protocol requires a TLS configuration.
func NeedsTLS(proto string) bool {
	return protocolNeedsTLS(proto)
}

func protocolNeedsTLS(proto string) bool {
	switch proto {
	case "vless", "trojan", "tuic", "anytls":
		return true
	default:
		return false
	}
}

func List(st *store.Store) []InventoryRow {
	if st == nil || st.Config == nil {
		return nil
	}
	rows := make([]InventoryRow, 0, len(st.Config.Inbounds)+1)
	for _, in := range st.Config.Inbounds {
		rows = append(rows, InventoryRow{
			Protocol: normalizeProtocolType(in.Type),
			Tag:      strings.TrimSpace(in.Tag),
			Port:     in.ListenPort,
			Users:    len(in.Users),
			Source:   "config",
		})
	}
	if st.SnellConf != nil {
		if p := parseListenPort(st.SnellConf.Get("listen")); p > 0 {
			users := 0
			if strings.TrimSpace(st.SnellConf.Get("psk")) != "" {
				users = 1
			}
			rows = append(rows, InventoryRow{
				Protocol: "snell",
				Tag:      "snell-v5",
				Port:     p,
				Users:    users,
				Source:   "snell",
			})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Protocol != rows[j].Protocol {
			return rows[i].Protocol < rows[j].Protocol
		}
		if rows[i].Tag != rows[j].Tag {
			return rows[i].Tag < rows[j].Tag
		}
		return rows[i].Port < rows[j].Port
	})
	return rows
}

type InventoryRow struct {
	Protocol string
	Tag      string
	Port     int
	Users    int
	Source   string
}
