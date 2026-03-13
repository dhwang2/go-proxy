package subscription

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/dhwang2/go-proxy/internal/store"
)

type OutputFormat string

const (
	FormatAll     OutputFormat = "all"
	FormatSingbox OutputFormat = "singbox"
	FormatSurge   OutputFormat = "surge"
)

type RenderOptions struct {
	User   string
	Host   string
	Format OutputFormat
}

type UserLinks struct {
	Singbox []string
	Surge   []string
}

type RenderResult struct {
	Target TargetSet
	Users  []string
	ByUser map[string]UserLinks
}

type protocolEntry struct {
	Protocol          string
	Tag               string
	Port              int
	SNI               string
	ALPN              string
	Method            string
	UserID            string
	Secret            string
	Flow              string
	RealityPrivateKey string
	RealityShortID    string
	Group             string
}

type inboundTLS struct {
	SNI            string
	ALPN           string
	RealityPrivKey string
	RealityShortID string
}

func ParseFormat(v string) (OutputFormat, error) {
	s := strings.ToLower(strings.TrimSpace(v))
	switch s {
	case "", "all":
		return FormatAll, nil
	case "singbox", "sb":
		return FormatSingbox, nil
	case "surge", "sg":
		return FormatSurge, nil
	default:
		return "", fmt.Errorf("invalid format %q (supported: all|singbox|surge)", v)
	}
}

func Render(st *store.Store, opts RenderOptions) (RenderResult, error) {
	if st == nil || st.Config == nil {
		return RenderResult{}, fmt.Errorf("store is nil")
	}
	if opts.Format == "" {
		opts.Format = FormatAll
	}
	target, err := DetectTarget(st, opts.Host)
	if err != nil {
		return RenderResult{}, err
	}
	nodeName := detectNodeName()
	filterUser := normalizeGroupName(opts.User)

	entries := collectProtocolEntries(st, filterUser)
	if snellEntry, ok := collectSnellEntry(st, filterUser); ok {
		entries = append(entries, snellEntry)
	}
	if len(entries) == 0 {
		return RenderResult{Target: target, Users: nil, ByUser: map[string]UserLinks{}}, nil
	}

	byUser := map[string]*UserLinks{}
	seenSingbox := map[string]map[string]struct{}{}
	seenSurge := map[string]map[string]struct{}{}

	for _, entry := range entries {
		user := entry.Group
		if user == "" {
			continue
		}
		if _, ok := byUser[user]; !ok {
			byUser[user] = &UserLinks{}
			seenSingbox[user] = map[string]struct{}{}
			seenSurge[user] = map[string]struct{}{}
		}

		if opts.Format == FormatAll || opts.Format == FormatSingbox {
			if line, ok := RenderSingbox(entry, target.Preferred, nodeName); ok {
				if _, dup := seenSingbox[user][line]; !dup {
					seenSingbox[user][line] = struct{}{}
					byUser[user].Singbox = append(byUser[user].Singbox, line)
				}
			}
		}
		if opts.Format == FormatAll || opts.Format == FormatSurge {
			for _, line := range RenderSurge(entry, target.Targets, nodeName) {
				if _, dup := seenSurge[user][line]; dup {
					continue
				}
				seenSurge[user][line] = struct{}{}
				byUser[user].Surge = append(byUser[user].Surge, line)
			}
		}
	}

	users := make([]string, 0, len(byUser))
	out := map[string]UserLinks{}
	for user, links := range byUser {
		if len(links.Singbox) == 0 && len(links.Surge) == 0 {
			continue
		}
		sort.Strings(links.Singbox)
		sort.Strings(links.Surge)
		users = append(users, user)
		out[user] = *links
	}
	sort.Strings(users)

	return RenderResult{Target: target, Users: users, ByUser: out}, nil
}

func collectProtocolEntries(st *store.Store, filterUser string) []protocolEntry {
	entries := make([]protocolEntry, 0)
	for _, in := range st.Config.Inbounds {
		proto := normalizeProto(in.Type)
		switch proto {
		case "vless", "tuic", "trojan", "anytls", "ss":
		default:
			continue
		}
		if in.ListenPort <= 0 || len(in.Users) == 0 {
			continue
		}
		tls := parseInboundTLS(in)
		fallbackMethod := readInboundString(in, "method")
		for _, u := range in.Users {
			entry := protocolEntry{
				Protocol:          proto,
				Tag:               strings.TrimSpace(in.Tag),
				Port:              in.ListenPort,
				SNI:               tls.SNI,
				ALPN:              tls.ALPN,
				RealityPrivateKey: tls.RealityPrivKey,
				RealityShortID:    tls.RealityShortID,
				UserID:            strings.TrimSpace(u.Key()),
				Secret:            strings.TrimSpace(u.Password),
				Method:            strings.TrimSpace(u.Method),
				Flow:              readRawString(u.Raw, "flow"),
			}
			if entry.Method == "" {
				entry.Method = fallbackMethod
			}
			entry.Group = resolveGroupName(st.UserMeta, proto, in.Tag, u)
			if filterUser != "" && entry.Group != filterUser {
				continue
			}
			if !entryValid(entry) {
				continue
			}
			entries = append(entries, entry)
		}
	}
	return entries
}

func collectSnellEntry(st *store.Store, filterUser string) (protocolEntry, bool) {
	if st == nil || st.SnellConf == nil {
		return protocolEntry{}, false
	}
	psk := strings.TrimSpace(st.SnellConf.Get("psk"))
	listen := strings.TrimSpace(st.SnellConf.Get("listen"))
	port := parsePortFromListen(listen)
	if psk == "" || port <= 0 {
		return protocolEntry{}, false
	}
	group := resolveGroupFromMeta(st.UserMeta, "snell", "snell-v5", psk, "")
	if filterUser != "" && group != filterUser {
		return protocolEntry{}, false
	}
	return protocolEntry{
		Protocol: "snell",
		Tag:      "snell-v5",
		Port:     port,
		UserID:   psk,
		Secret:   psk,
		Group:    group,
	}, true
}

func entryValid(e protocolEntry) bool {
	switch e.Protocol {
	case "vless":
		return e.UserID != ""
	case "tuic":
		return e.UserID != "" && e.Secret != ""
	case "trojan", "anytls":
		return e.Secret != ""
	case "ss":
		return e.Method != "" && e.Secret != ""
	default:
		return false
	}
}

func parsePortFromListen(v string) int {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if strings.Contains(v, ":") {
		parts := strings.Split(v, ":")
		v = parts[len(parts)-1]
	}
	p, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || p <= 0 {
		return 0
	}
	return p
}

func normalizeProto(v string) string {
	s := strings.ToLower(strings.TrimSpace(v))
	if s == "shadowsocks" {
		return "ss"
	}
	return s
}

func detectNodeName() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		return "go-proxy"
	}
	host = strings.ToLower(strings.TrimSpace(host))
	replacer := strings.NewReplacer(" ", "-", "_", "-", ".", "-")
	host = replacer.Replace(host)
	for strings.Contains(host, "--") {
		host = strings.ReplaceAll(host, "--", "-")
	}
	host = strings.Trim(host, "-")
	if host == "" {
		return "go-proxy"
	}
	return host
}

func parseInboundTLS(in store.Inbound) inboundTLS {
	if in.Raw == nil {
		return inboundTLS{}
	}
	tlsRaw, ok := in.Raw["tls"]
	if !ok || len(tlsRaw) == 0 {
		return inboundTLS{}
	}
	var m map[string]any
	if err := json.Unmarshal(tlsRaw, &m); err != nil {
		return inboundTLS{}
	}
	out := inboundTLS{SNI: readAnyString(m["server_name"])}
	out.ALPN = strings.Join(readAnyStringList(m["alpn"]), ",")
	if reality, ok := m["reality"].(map[string]any); ok {
		out.RealityPrivKey = readAnyString(reality["private_key"])
		shortIDs := readAnyStringList(reality["short_id"])
		if len(shortIDs) > 0 {
			out.RealityShortID = shortIDs[0]
		}
	}
	return out
}

func readInboundString(in store.Inbound, key string) string {
	if in.Raw == nil {
		return ""
	}
	return readRawString(in.Raw, key)
}

func readRawString(raw map[string]json.RawMessage, key string) string {
	if raw == nil {
		return ""
	}
	b, ok := raw[key]
	if !ok || len(b) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		return strings.TrimSpace(s)
	}
	return ""
}

func readAnyString(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func readAnyStringList(v any) []string {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return nil
		}
		return []string{s}
	case []any:
		out := make([]string, 0, len(x))
		for _, it := range x {
			s, ok := it.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			out = append(out, s)
		}
		return out
	default:
		return nil
	}
}

func resolveGroupName(meta *store.UserMeta, proto, tag string, user store.User) string {
	fallback := strings.TrimSpace(user.Name)
	id := strings.TrimSpace(user.Key())
	return resolveGroupFromMeta(meta, proto, tag, id, fallback)
}

func resolveGroupFromMeta(meta *store.UserMeta, proto, tag, userID, fallback string) string {
	if meta != nil {
		key := userMetaKey(proto, tag, userID)
		if name := normalizeGroupName(meta.Name[key]); name != "" {
			return name
		}
		if d, ok := meta.Disabled[key]; ok {
			if name := normalizeGroupName(d.User.Name); name != "" {
				return name
			}
		}
	}
	if name := normalizeGroupName(fallback); name != "" {
		return name
	}
	return "user"
}

func userMetaKey(proto, tag, userID string) string {
	proto = normalizeProto(proto)
	return proto + "|" + strings.TrimSpace(tag) + "|" + strings.TrimSpace(userID)
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
	v = strings.Trim(v, "-_.")
	if v == "" {
		return "user"
	}
	return v
}
