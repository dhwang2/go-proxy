package subscription

import (
	"fmt"

	"go-proxy/internal/crypto"
	"go-proxy/internal/derived"
	"go-proxy/internal/store"
)

// renderSurge generates a Surge-format proxy line for an inbound membership.
func renderSurge(ib *store.Inbound, entry derived.MembershipEntry, host string) string {
	fmtHost := FormatHost(host)
	sni := ib.ServerName()

	switch ib.Type {
	case "vless":
		if ib.HasReality() {
			return fmt.Sprintf("%s = vless, %s, %d, username=%s, sni=%s, tls=true, reality=true",
				entry.Tag, fmtHost, ib.ListenPort, entry.UserID, sni)
		}
		return fmt.Sprintf("%s = vless, %s, %d, username=%s, sni=%s, tls=true",
			entry.Tag, fmtHost, ib.ListenPort, entry.UserID, sni)

	case "tuic":
		return fmt.Sprintf("%s = tuic-v5, %s, %d, token=%s, sni=%s, alpn=h3",
			entry.Tag, fmtHost, ib.ListenPort, entry.UserID, sni)

	case "trojan":
		if ib.HasReality() {
			return fmt.Sprintf("%s = trojan, %s, %d, password=%s, sni=%s, tls=true, reality=true",
				entry.Tag, fmtHost, ib.ListenPort, entry.UserID, sni)
		}
		return fmt.Sprintf("%s = trojan, %s, %d, password=%s, sni=%s",
			entry.Tag, fmtHost, ib.ListenPort, entry.UserID, sni)

	case "anytls":
		return fmt.Sprintf("%s = anytls, %s, %d, password=%s, sni=%s",
			entry.Tag, fmtHost, ib.ListenPort, entry.UserID, sni)

	case "shadowsocks":
		method := ib.Method
		if method == "" {
			method = crypto.DefaultSSMethod
		}
		return fmt.Sprintf("%s = ss, %s, %d, encrypt-method=%s, password=%s",
			entry.Tag, fmtHost, ib.ListenPort, method, ssPassword(ib, entry.UserID))

	default:
		return fmt.Sprintf("# unsupported protocol: %s", ib.Type)
	}
}
