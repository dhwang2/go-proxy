package subscription

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"go-proxy/internal/derived"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
)

// renderSurge generates a Surge-format proxy line for an inbound membership.
// targetHost is the IP/host to connect to; sniHost is used for SNI (original domain or configured SNI).
// tagSuffix is appended to the proxy tag (e.g. "-v4", "-v6", or "").
func renderSurge(ib *store.Inbound, entry derived.MembershipEntry, targetHost, sniHost, tag string, user *store.User) string {
	fmtHost := targetHost
	sni := ib.ServerName()
	if sni == "" {
		cleaned := SanitizeServerName(sniHost)
		// Only use domain names for SNI, never IP literals.
		if net.ParseIP(cleaned) == nil {
			sni = cleaned
		}
	}

	switch ib.Type {
	case "vless":
		return ""

	case "tuic":
		if user == nil || user.UUID == "" || user.Password == "" {
			return ""
		}
		uuid := user.UUID
		if looksLikeUUID(uuid) {
			uuid = strings.ToUpper(uuid)
		}
		// Only what Surge's tuic-v5 defines. Congestion control is the
		// server's choice and has no Surge parameter, and UDP relay is always
		// on for TUIC, so neither is written.
		return fmt.Sprintf("%s = tuic-v5, %s, %d, password=%s, uuid=%s, alpn=h3, sni=%s, skip-cert-verify=false",
			tag, fmtHost, ib.ListenPort, user.Password, uuid, sni)

	case "anytls":
		return fmt.Sprintf("%s = anytls, %s, %d, password=%s, sni=%s, skip-cert-verify=false, reuse=true",
			tag, fmtHost, ib.ListenPort, entry.UserID, sni)

	default:
		return ""
	}
}

func renderSnellSurge(entry derived.MembershipEntry, conf *store.SnellConfig, targetHost, tag string) string {
	if conf == nil || conf.PSK == "" {
		return ""
	}
	return fmt.Sprintf("%s = snell, %s, %d, psk=%s, version=6, mode=%s, reuse=true, tfo=true",
		tag, targetHost, conf.Port(), conf.PSK, snellMode(conf))
}

func renderShadowTLSSnellSurge(entry derived.MembershipEntry, conf *store.SnellConfig, binding service.ShadowTLSBinding, targetHost, tag string) string {
	if conf == nil || conf.PSK == "" {
		return ""
	}
	version := binding.Version
	if version == 0 {
		version = 3
	}
	return fmt.Sprintf("%s = snell, %s, %d, psk=%s, version=6, mode=%s, reuse=true, tfo=true, shadow-tls-password=%s, shadow-tls-sni=%s, shadow-tls-version=%s",
		tag, targetHost, binding.ListenPort, conf.PSK, snellMode(conf), binding.Password, binding.SNI, strconv.Itoa(version))
}

// snellMode is the server's mode, which the client must match: Surge
// assumes default, so a server set otherwise is unreachable without it.
func snellMode(conf *store.SnellConfig) string {
	mode, _ := conf.Settings()
	return mode
}

var (
	serverNameOnce  sync.Once
	serverNameValue string
)

var serverName = func() string {
	serverNameOnce.Do(func() {
		name, _ := os.Hostname()
		if idx := strings.IndexByte(name, '.'); idx > 0 {
			name = name[:idx]
		}
		serverNameValue = name
	})
	return serverNameValue
}

// proxyName builds the name a client shows for one link: the host, the
// protocol, the address family and the owner. The node tag is not in it -- a
// tag begins with its own protocol, so including it named the protocol twice
// ("gcp-oregon-snell-snell-v6-v4-alice").
//
// A port is added only where it is needed to tell two links apart, which is
// when one user has more than one node of the same protocol. Names key the
// proxy list in every client that reads these, so two links sharing one would
// silently replace each other.
func proxyName(proto, userName, family string, needsPort bool, port int) string {
	parts := []string{}
	if name := serverName(); name != "" {
		parts = append(parts, name)
	}
	parts = append(parts, proto)
	if needsPort {
		parts = append(parts, strconv.Itoa(port))
	}
	if family != "" {
		parts = append(parts, family)
	}
	return strings.Join(append(parts, userName), "-")
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
