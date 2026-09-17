package subscription

import (
	"encoding/base64"
	"fmt"
	"net/url"

	"go-proxy/internal/crypto"
	"go-proxy/internal/derived"
	"go-proxy/internal/store"
)

// uriFragment returns the URI fragment (#name) using a unique inbound-aware format.
func uriFragment(ibType, userName, tag string) string {
	proto := surgeProtoLabel(ibType)
	return surgeProxyTag(proto, userName, tag)
}

// renderURI generates a protocol share URI for an inbound membership.
func renderURI(ib *store.Inbound, entry derived.MembershipEntry, host string, u *store.User, tls *clientTLS) string {
	fmtHost := FormatHost(host)
	sni := ib.ServerName()
	fragment := uriFragment(ib.Type, entry.UserName, entry.Tag)

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
		return fmt.Sprintf("vless://%s@%s:%d?%s#%s",
			entry.UserID, fmtHost, ib.ListenPort, params.Encode(), fragment)

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
		params.Set("congestion_control", congestion)
		params.Set("alpn", "h3")
		params.Set("sni", sni)
		params.Set("udp_relay_mode", "native")
		params.Set("allow_insecure", "0")
		return fmt.Sprintf("tuic://%s:%s@%s:%d?%s#%s",
			entry.UserID, password, fmtHost, ib.ListenPort, params.Encode(), fragment)

	case "anytls":
		params := url.Values{}
		params.Set("sni", sni)
		return fmt.Sprintf("anytls://%s@%s:%d?%s#%s",
			entry.UserID, fmtHost, ib.ListenPort, params.Encode(), fragment)

	case "shadowsocks":
		method := ib.Method
		if method == "" {
			method = crypto.DefaultSSMethod
		}
		password := ssPassword(ib, entry.UserID)
		auth := base64.RawURLEncoding.EncodeToString([]byte(method + ":" + password))
		return fmt.Sprintf("ss://%s@%s:%d#%s",
			auth, fmtHost, ib.ListenPort, fragment)

	default:
		return fmt.Sprintf("# unsupported: %s", ib.Type)
	}
}
