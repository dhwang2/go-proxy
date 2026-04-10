package subscription

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

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
func renderURI(ib *store.Inbound, entry derived.MembershipEntry, host string) string {
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
					if len(r.ShortID) > 0 {
						params.Set("sid", r.ShortID[0])
					}
				}
			} else {
				params.Set("security", "tls")
			}
			params.Set("sni", sni)
		}
		if u := ib.FindUser(entry.UserName); u != nil && u.Flow != "" {
			params.Set("flow", u.Flow)
		}
		return fmt.Sprintf("vless://%s@%s:%d?%s#%s",
			entry.UserID, fmtHost, ib.ListenPort, params.Encode(), fragment)

	case "tuic":
		password := ""
		if u := ib.FindUser(entry.UserName); u != nil {
			password = u.Password
		}
		params := url.Values{}
		params.Set("congestion_control", "bbr")
		params.Set("alpn", "h3")
		params.Set("sni", sni)
		params.Set("udp_relay_mode", "native")
		params.Set("allow_insecure", "1")
		return fmt.Sprintf("tuic://%s:%s@%s:%d?%s#%s",
			entry.UserID, password, fmtHost, ib.ListenPort, params.Encode(), fragment)

	case "trojan":
		params := url.Values{}
		params.Set("security", "tls")
		params.Set("sni", sni)
		params.Set("type", "tcp")
		if ib.TLS != nil && len(ib.TLS.ALPN) > 0 {
			params.Set("alpn", strings.Join(ib.TLS.ALPN, ","))
		}
		return fmt.Sprintf("trojan://%s@%s:%d?%s#%s",
			entry.UserID, fmtHost, ib.ListenPort, params.Encode(), fragment)

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
