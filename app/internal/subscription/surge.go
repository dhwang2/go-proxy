package subscription

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"go-proxy/internal/crypto"
	"go-proxy/internal/derived"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
)

// renderSurge generates a Surge-format proxy line for an inbound membership.
// targetHost is the IP/host to connect to; sniHost is used for SNI (original domain or configured SNI).
// tagSuffix is appended to the proxy tag (e.g. "-v4", "-v6", or "").
func renderSurge(ib *store.Inbound, entry derived.MembershipEntry, targetHost, sniHost, tagSuffix string) string {
	fmtHost := FormatHost(targetHost)
	sni := ib.ServerName()
	if sni == "" {
		cleaned := SanitizeServerName(sniHost)
		// Only use domain names for SNI, never IP literals.
		if net.ParseIP(cleaned) == nil {
			sni = cleaned
		}
	}
	tag := surgeProxyTag(surgeProtoLabel(ib.Type), entry.UserName, tagSuffix)

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
			tag, fmtHost, ib.ListenPort, user.Password, uuid, sni)

	case "trojan":
		params := []string{
			fmt.Sprintf("%s = trojan, %s, %d, password=%s, sni=%s", tag, fmtHost, ib.ListenPort, entry.UserID, sni),
		}
		if alpn := firstALPN(ib.TLS); alpn != "" {
			params = append(params, "alpn="+alpn)
		}
		params = append(params, "skip-cert-verify=false", "udp-relay=true")
		return strings.Join(params, ", ")

	case "anytls":
		return fmt.Sprintf("%s = anytls, %s, %d, password=%s, sni=%s, skip-cert-verify=false, reuse=true",
			tag, fmtHost, ib.ListenPort, entry.UserID, sni)

	case "shadowsocks":
		method := ib.Method
		if method == "" {
			method = crypto.DefaultSSMethod
		}
		return fmt.Sprintf("%s = ss, %s, %d, encrypt-method=%s, password=\"%s\", udp-relay=true",
			tag, fmtHost, ib.ListenPort, method, escapeSurgeQuoted(ssPassword(ib, entry.UserID)))

	default:
		return ""
	}
}

func renderSnellSurge(entry derived.MembershipEntry, conf *store.SnellConfig, targetHost, tagSuffix string) string {
	if conf == nil || conf.PSK == "" {
		return ""
	}
	tag := surgeProxyTag("snell", entry.UserName, tagSuffix)
	return fmt.Sprintf("%s = snell, %s, %d, psk=%s, version=5, reuse=true, tfo=true",
		tag, FormatHost(targetHost), conf.Port(), conf.PSK)
}

func renderShadowTLSShadowsocksSurge(ib *store.Inbound, entry derived.MembershipEntry, binding service.ShadowTLSBinding, targetHost, tagSuffix string) string {
	method := ib.Method
	if method == "" {
		method = crypto.DefaultSSMethod
	}
	version := binding.Version
	if version == 0 {
		version = 3
	}
	tag := surgeProxyTag("ss", entry.UserName, tagSuffix)
	return fmt.Sprintf("%s = ss, %s, %d, encrypt-method=%s, password=\"%s\", shadow-tls-password=%s, shadow-tls-sni=%s, shadow-tls-version=%s, udp-relay=true",
		tag, FormatHost(targetHost), binding.ListenPort, method, escapeSurgeQuoted(ssPassword(ib, entry.UserID)), binding.Password, binding.SNI, strconv.Itoa(version))
}

func renderShadowTLSSnellSurge(entry derived.MembershipEntry, conf *store.SnellConfig, binding service.ShadowTLSBinding, targetHost, tagSuffix string) string {
	if conf == nil || conf.PSK == "" {
		return ""
	}
	version := binding.Version
	if version == 0 {
		version = 3
	}
	tag := surgeProxyTag("snell", entry.UserName, tagSuffix)
	return fmt.Sprintf("%s = snell, %s, %d, psk=%s, version=5, reuse=true, tfo=true, shadow-tls-password=%s, shadow-tls-sni=%s, shadow-tls-version=%s",
		tag, FormatHost(targetHost), binding.ListenPort, conf.PSK, binding.Password, binding.SNI, strconv.Itoa(version))
}

// surgeProtoLabel returns a short, user-facing protocol label for surge proxy names.
func surgeProtoLabel(ibType string) string {
	if ibType == "shadowsocks" {
		return "ss"
	}
	return ibType
}

var (
	serverNameOnce  sync.Once
	serverNameValue string
)

func serverName() string {
	serverNameOnce.Do(func() {
		name, _ := os.Hostname()
		if idx := strings.IndexByte(name, '.'); idx > 0 {
			name = name[:idx]
		}
		serverNameValue = name
	})
	return serverNameValue
}

func surgeProxyTag(proto, userName, tagSuffix string) string {
	sn := serverName()
	suffix := strings.TrimPrefix(tagSuffix, "-")
	parts := []string{}
	if sn != "" {
		parts = append(parts, sn)
	}
	parts = append(parts, proto)
	if suffix != "" {
		parts = append(parts, suffix)
	}
	parts = append(parts, userName)
	return strings.Join(parts, "-")
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
