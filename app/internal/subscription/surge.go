package subscription

import (
	"fmt"
	"strings"

	"go-proxy/internal/crypto"
	"go-proxy/internal/derived"
	"go-proxy/internal/store"
)

// renderSurge generates a Surge-format proxy line for an inbound membership.
func renderSurge(ib *store.Inbound, entry derived.MembershipEntry, host string) string {
	fmtHost := FormatHost(host)
	sni := ib.ServerName()
	if sni == "" {
		sni = SanitizeServerName(host)
	}

	switch ib.Type {
	case "vless":
		return ""

	case "tuic":
		user := ib.FindUser(entry.UserName)
		if user == nil || user.UUID == "" || user.Password == "" {
			return ""
		}
		uuid := user.UUID
		if looksLikeUUID(uuid) {
			uuid = strings.ToUpper(uuid)
		}
		return fmt.Sprintf("%s = tuic-v5, %s, %d, password=%s, uuid=%s, alpn=h3, sni=%s, skip-cert-verify=false, congestion-controller=bbr, udp-relay=true",
			entry.Tag, fmtHost, ib.ListenPort, user.Password, uuid, sni)

	case "trojan":
		params := []string{
			fmt.Sprintf("%s = trojan, %s, %d, password=%s, sni=%s", entry.Tag, fmtHost, ib.ListenPort, entry.UserID, sni),
		}
		if alpn := firstALPN(ib.TLS); alpn != "" {
			params = append(params, "alpn="+alpn)
		}
		params = append(params, "skip-cert-verify=false", "udp-relay=true")
		return strings.Join(params, ", ")

	case "anytls":
		return fmt.Sprintf("%s = anytls, %s, %d, password=%s, sni=%s, skip-cert-verify=false, reuse=true",
			entry.Tag, fmtHost, ib.ListenPort, entry.UserID, sni)

	case "shadowsocks":
		method := ib.Method
		if method == "" {
			method = crypto.DefaultSSMethod
		}
		return fmt.Sprintf("%s = ss, %s, %d, encrypt-method=%s, password=\"%s\", udp-relay=true",
			entry.Tag, fmtHost, ib.ListenPort, method, escapeSurgeQuoted(ssPassword(ib, entry.UserID)))

	default:
		return ""
	}
}

func renderSnellSurge(entry derived.MembershipEntry, conf *store.SnellConfig, host string) string {
	if conf == nil || conf.PSK == "" {
		return ""
	}
	return fmt.Sprintf("%s = snell, %s, %d, psk=%s, version=5, reuse=true, tfo=true",
		entry.Tag, FormatHost(host), conf.Port(), conf.PSK)
}

func firstALPN(tls *store.TLSConfig) string {
	if tls == nil || len(tls.ALPN) == 0 {
		return ""
	}
	for _, alpn := range tls.ALPN {
		if trimmed := strings.TrimSpace(alpn); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func escapeSurgeQuoted(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}

func looksLikeUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
				return false
			}
		}
	}
	return true
}
