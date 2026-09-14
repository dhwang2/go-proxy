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

	case "tuic":
		out["uuid"] = entry.UserID
		if u := ib.FindUser(entry.UserName); u != nil && u.Password != "" {
			out["password"] = u.Password
		}
		out["congestion_control"] = "bbr"

	case "anytls":
		out["password"] = entry.UserID

	case "shadowsocks":
		method := ib.Method
		if method == "" {
			method = crypto.DefaultSSMethod
		}
		out["method"] = method
		out["password"] = ssPassword(ib, entry.UserID)
	default:
		return ""
	}
	if ib.TLS != nil {
		tls, err := buildClientTLS(ib.TLS)
		if err != nil {
			return ""
		}
		out["tls"] = tls
	}

	data, _ := json.MarshalIndent(out, "", "  ")
	return string(data)
}

func buildClientTLS(tls *store.TLSConfig) (map[string]any, error) {
	t := map[string]any{
		"enabled":     true,
		"server_name": tls.ServerName,
	}
	if len(tls.ALPN) > 0 {
		t["alpn"] = tls.ALPN
	}
	if tls.Reality != nil && tls.Reality.Enabled {
		publicKey, err := crypto.RealityPublicKey(tls.Reality.PrivateKey)
		if err != nil {
			return nil, err
		}
		reality := map[string]any{
			"enabled":    true,
			"public_key": publicKey,
		}
		if len(tls.Reality.ShortID) > 0 {
			reality["short_id"] = tls.Reality.ShortID[0]
		}
		t["reality"] = reality
		t["utls"] = map[string]any{"enabled": true, "fingerprint": "chrome"}
	}
	return t, nil
}

// ssPassword composes the Shadowsocks 2022 password (server_key:user_key for multi-user).
func ssPassword(ib *store.Inbound, userKey string) string {
	if ib.Password != "" {
		return fmt.Sprintf("%s:%s", ib.Password, userKey)
	}
	return userKey
}
