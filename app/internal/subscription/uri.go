package subscription

import (
	"fmt"
	"net"
	"net/url"
	"strconv"

	"go-proxy/internal/derived"
	"go-proxy/internal/store"
)

// shareLink assembles scheme://auth@host:port/?query#name. The credentials and
// the name are percent-encoded, as the anytls and TUIC share schemes require:
// a password holding '@', ':' or '/' would otherwise end the authority early.
// The empty path is written as "/" because the schemes' own examples do.
func shareLink(scheme string, auth *url.Userinfo, host string, port int, params url.Values, name string) string {
	link := url.URL{
		Scheme:   scheme,
		User:     auth,
		Host:     net.JoinHostPort(host, strconv.Itoa(port)),
		Path:     "/",
		RawQuery: params.Encode(),
		Fragment: name,
	}
	return link.String()
}

// renderURI generates a protocol share URI for an inbound membership.
func renderURI(ib *store.Inbound, entry derived.MembershipEntry, host, fragment string, u *store.User, tls *clientTLS) string {
	sni := ib.ServerName()

	switch ib.Type {
	case "vless":
		params := url.Values{}
		params.Set("encryption", "none")
		params.Set("type", "tcp")
		if ib.TLS != nil {
			if ib.HasReality() {
				params.Set("security", "reality")
				params.Set("fp", "chrome")
				if r := ib.TLS.Reality; r != nil {
					params.Set("pbk", tls.Reality.PublicKey)
					if len(r.ShortID) > 0 {
						params.Set("sid", r.ShortID[0])
					}
				}
			} else {
				params.Set("security", "tls")
			}
			params.Set("sni", sni)
		}
		if u != nil && u.Flow != "" {
			params.Set("flow", u.Flow)
		}
		return shareLink("vless", url.User(entry.UserID), host, ib.ListenPort, params, fragment)

	case "tuic":
		password := ""
		if u != nil {
			password = u.Password
		}
		params := url.Values{}
		congestion := ib.CongestionControl
		if congestion == "" {
			congestion = "bbr"
		}
		// The parameters Mihomo's share-link parser reads. It dropped
		// allow_insecure from the scheme it follows, and certificate
		// verification is on unless a link says otherwise.
		params.Set("congestion_control", congestion)
		params.Set("alpn", "h3")
		params.Set("sni", sni)
		params.Set("udp_relay_mode", "native")
		return shareLink("tuic", url.UserPassword(entry.UserID, password), host, ib.ListenPort, params, fragment)

	case "anytls":
		// https://github.com/anytls/anytls-go/blob/main/docs/uri_scheme.md
		params := url.Values{}
		params.Set("sni", sni)
		return shareLink("anytls", url.User(entry.UserID), host, ib.ListenPort, params, fragment)

	default:
		return fmt.Sprintf("# unsupported: %s", ib.Type)
	}
}
