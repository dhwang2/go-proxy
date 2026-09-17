package subscription

import (
	"encoding/json"

	"go-proxy/internal/crypto"
	"go-proxy/internal/derived"
	"go-proxy/internal/store"
)

type clientOutbound struct {
	Type       string     `json:"type"`
	Tag        string     `json:"tag"`
	Server     string     `json:"server"`
	ServerPort int        `json:"server_port"`
	UUID       string     `json:"uuid,omitempty"`
	Flow       string     `json:"flow,omitempty"`
	Password   string     `json:"password,omitempty"`
	Method     string     `json:"method,omitempty"`
	Congestion string     `json:"congestion_control,omitempty"`
	TLS        *clientTLS `json:"tls,omitempty"`
}

type clientTLS struct {
	Enabled    bool           `json:"enabled"`
	ServerName string         `json:"server_name"`
	ALPN       []string       `json:"alpn,omitempty"`
	Reality    *clientReality `json:"reality,omitempty"`
	UTLS       *clientUTLS    `json:"utls,omitempty"`
}

type clientReality struct {
	Enabled   bool   `json:"enabled"`
	PublicKey string `json:"public_key"`
	ShortID   string `json:"short_id,omitempty"`
}

type clientUTLS struct {
	Enabled     bool   `json:"enabled"`
	Fingerprint string `json:"fingerprint"`
}

func renderSingBox(ib *store.Inbound, entry derived.MembershipEntry, host string, u *store.User, tls *clientTLS) string {
	out := clientOutbound{Type: ib.Type, Tag: entry.Tag, Server: host, ServerPort: ib.ListenPort, TLS: tls}
	switch ib.Type {
	case "vless":
		out.UUID = entry.UserID
		if u != nil {
			out.Flow = u.Flow
		}
	case "tuic":
		out.UUID = entry.UserID
		if u != nil {
			out.Password = u.Password
		}
		out.Congestion = ib.CongestionControl
		if out.Congestion == "" {
			out.Congestion = "bbr"
		}
	case "anytls":
		out.Password = entry.UserID
	case "shadowsocks":
		out.Method = ib.Method
		if out.Method == "" {
			out.Method = crypto.DefaultSSMethod
		}
		out.Password = ssPassword(ib, entry.UserID)
	default:
		return ""
	}
	data, _ := json.Marshal(out)
	return string(data)
}

func buildClientTLS(tls *store.TLSConfig) (*clientTLS, error) {
	t := &clientTLS{Enabled: true, ServerName: tls.ServerName, ALPN: tls.ALPN}
	if tls.Reality != nil && tls.Reality.Enabled {
		publicKey, err := crypto.RealityPublicKey(tls.Reality.PrivateKey)
		if err != nil {
			return nil, err
		}
		t.Reality = &clientReality{Enabled: true, PublicKey: publicKey}
		if len(tls.Reality.ShortID) > 0 {
			t.Reality.ShortID = tls.Reality.ShortID[0]
		}
		t.UTLS = &clientUTLS{Enabled: true, Fingerprint: "chrome"}
	}
	return t, nil
}

func ssPassword(ib *store.Inbound, userKey string) string {
	if ib.Password != "" {
		return ib.Password + ":" + userKey
	}
	return userKey
}
