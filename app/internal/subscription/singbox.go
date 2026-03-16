package subscription

import (
	"encoding/json"
	"fmt"

	"go-proxy/internal/crypto"
	"go-proxy/internal/derived"
	"go-proxy/internal/store"
)

// renderSingBox generates a sing-box client outbound JSON for an inbound membership.
func renderSingBox(ib *store.Inbound, entry derived.MembershipEntry, host string) string {
	out := map[string]any{
		"type":        ib.Type,
		"tag":         entry.Tag,
		"server":      host,
		"server_port": ib.ListenPort,
	}

	switch ib.Type {
	case "vless":
		out["uuid"] = entry.UserID
		if u := ib.FindUser(entry.UserName); u != nil && u.Flow != "" {
			out["flow"] = u.Flow
		}
		if ib.TLS != nil {
			out["tls"] = buildClientTLS(ib.TLS)
		}

	case "tuic":
		out["uuid"] = entry.UserID
		if u := ib.FindUser(entry.UserName); u != nil && u.Password != "" {
			out["password"] = u.Password
		}
		out["congestion_control"] = "bbr"
		if ib.TLS != nil {
			out["tls"] = buildClientTLS(ib.TLS)
		}

	case "trojan":
		out["password"] = entry.UserID
		if ib.TLS != nil {
			out["tls"] = buildClientTLS(ib.TLS)
		}

	case "anytls":
		out["password"] = entry.UserID
		if ib.TLS != nil {
			out["tls"] = buildClientTLS(ib.TLS)
		}

	case "shadowsocks":
		method := ib.Method
		if method == "" {
			method = crypto.DefaultSSMethod
		}
		out["method"] = method
		out["password"] = ssPassword(ib, entry.UserID)
	}

	data, _ := json.MarshalIndent(out, "", "  ")
	return string(data)
}

func buildClientTLS(tls *store.TLSConfig) map[string]any {
	t := map[string]any{
		"enabled":     true,
		"server_name": tls.ServerName,
	}
	if len(tls.ALPN) > 0 {
		t["alpn"] = tls.ALPN
	}
	if tls.Reality != nil && tls.Reality.Enabled {
		reality := map[string]any{
			"enabled": true,
		}
		if len(tls.Reality.ShortID) > 0 {
			reality["short_id"] = tls.Reality.ShortID[0]
		}
		t["reality"] = reality
	}
	return t
}

// ssPassword composes the Shadowsocks 2022 password (server_key:user_key for multi-user).
func ssPassword(ib *store.Inbound, userKey string) string {
	if ib.Password != "" {
		return fmt.Sprintf("%s:%s", ib.Password, userKey)
	}
	return userKey
}
