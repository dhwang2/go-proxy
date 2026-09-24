package subscription

import (
	"encoding/json"
	"strconv"
	"strings"

	"go-proxy/internal/derived"
	"go-proxy/internal/store"
)

// Mihomo proxies are emitted as flow-style YAML, one node per line. YAML flow
// mappings are a superset of JSON, so every string is written with the JSON
// encoder and needs no separate escaping pass, while the keys stay unquoted so
// the result still reads like the configuration a person edits by hand.
//
// Snell is deliberately absent. Mihomo implements Snell v1 to v5; this project
// deploys snell-server v6, whose wire format derives a per-deployment profile
// from the PSK. A v5 entry would load and then fail to connect, which is worse
// than an export that says the node was skipped.

type field struct {
	key   string
	value any
}

func flow(fields []field) string {
	var b strings.Builder
	b.WriteByte('{')
	first := true
	for _, f := range fields {
		if f.value == nil {
			continue
		}
		if text, ok := f.value.(string); ok && text == "" {
			continue
		}
		if !first {
			// No space after the comma; a space after the colon stays. YAML
			// reads "{name:\"a\"}" as one key named `name:"a"` with a null
			// value -- it parses, and everything is wrong.
			b.WriteByte(',')
		}
		first = false
		b.WriteString(f.key)
		b.WriteString(": ")
		b.WriteString(scalar(f.value))
	}
	b.WriteByte('}')
	return b.String()
}

func scalar(value any) string {
	switch typed := value.(type) {
	case string:
		encoded, _ := json.Marshal(typed)
		return string(encoded)
	case int:
		return strconv.Itoa(typed)
	case bool:
		return strconv.FormatBool(typed)
	case []string:
		quoted := make([]string, 0, len(typed))
		for _, item := range typed {
			quoted = append(quoted, scalar(item))
		}
		return "[" + strings.Join(quoted, ", ") + "]"
	case []field:
		return flow(typed)
	}
	return "null"
}

// renderMihomo builds one proxy entry. It returns "" for a node Mihomo cannot
// represent, which the caller reports rather than emitting a broken entry.
func renderMihomo(ib *store.Inbound, entry derived.MembershipEntry, host, name string, u *store.User, tls *clientTLS) string {
	common := []field{
		{"name", name},
		{"type", ib.Type},
		{"server", host},
		{"port", ib.ListenPort},
	}
	sni := ib.ServerName()

	switch ib.Type {
	case "vless":
		out := append(common,
			field{"uuid", entry.UserID},
			field{"udp", true},
			field{"tls", ib.TLS != nil},
			field{"network", "tcp"},
		)
		if u != nil && u.Flow != "" {
			out = append(out, field{"flow", u.Flow})
		}
		if ib.TLS != nil {
			out = append(out, field{"servername", sni}, field{"client-fingerprint", "chrome"})
			if ib.HasReality() && tls != nil && tls.Reality != nil {
				reality := []field{{"public-key", tls.Reality.PublicKey}}
				if tls.Reality.ShortID != "" {
					reality = append(reality, field{"short-id", tls.Reality.ShortID})
				}
				out = append(out, field{"reality-opts", reality})
			} else {
				out = append(out, field{"skip-cert-verify", false})
			}
		}
		return flow(out)

	case "tuic":
		password := ""
		if u != nil {
			password = u.Password
		}
		if password == "" {
			return ""
		}
		congestion := ib.CongestionControl
		if congestion == "" {
			congestion = "bbr"
		}
		return flow(append(common,
			field{"uuid", entry.UserID},
			field{"password", password},
			field{"sni", sni},
			field{"alpn", []string{"h3"}},
			field{"congestion-controller", congestion},
			field{"udp", true},
			field{"udp-relay-mode", "native"},
			field{"reduce-rtt", true},
			field{"skip-cert-verify", false},
		))

	case "anytls":
		return flow(append(common,
			field{"password", entry.UserID},
			field{"sni", sni},
			field{"udp", true},
			field{"client-fingerprint", "chrome"},
			field{"skip-cert-verify", false},
		))

	}
	return ""
}
