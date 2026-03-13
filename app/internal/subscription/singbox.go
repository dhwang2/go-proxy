package subscription

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/dhwang2/go-proxy/internal/crypto"
)

func RenderSingbox(entry protocolEntry, target Target, nodeName string) (string, bool) {
	hostPort := net.JoinHostPort(target.Host, strconv.Itoa(entry.Port))
	fragment := fmt.Sprintf("%s_%s_%d_%s", nodeName, entry.Protocol, entry.Port, entry.Group)

	switch entry.Protocol {
	case "vless":
		if entry.UserID == "" {
			return "", false
		}
		q := url.Values{}
		q.Set("encryption", "none")
		q.Set("security", "reality")
		q.Set("type", "tcp")
		if entry.SNI != "" {
			q.Set("sni", entry.SNI)
		}
		if entry.ALPN != "" {
			q.Set("alpn", entry.ALPN)
		}
		q.Set("fp", "chrome")
		flow := entry.Flow
		if flow == "" {
			flow = "xtls-rprx-vision"
		}
		q.Set("flow", flow)
		if entry.RealityPrivateKey != "" {
			pub, err := crypto.DeriveRealityPublicKey(entry.RealityPrivateKey)
			if err != nil || pub == "" {
				return "", false
			}
			q.Set("pbk", pub)
		}
		if entry.RealityShortID != "" {
			q.Set("sid", entry.RealityShortID)
		}
		u := url.URL{
			Scheme:   "vless",
			User:     url.User(entry.UserID),
			Host:     hostPort,
			RawQuery: q.Encode(),
			Fragment: fragment,
		}
		return u.String(), true
	case "tuic":
		if entry.UserID == "" || entry.Secret == "" {
			return "", false
		}
		q := url.Values{}
		q.Set("congestion_control", "bbr")
		q.Set("alpn", "h3")
		if entry.SNI != "" {
			q.Set("sni", entry.SNI)
		}
		q.Set("udp_relay_mode", "native")
		q.Set("allow_insecure", "1")
		u := url.URL{
			Scheme:   "tuic",
			User:     url.UserPassword(entry.UserID, entry.Secret),
			Host:     hostPort,
			RawQuery: q.Encode(),
			Fragment: fragment,
		}
		return u.String(), true
	case "trojan":
		if entry.Secret == "" {
			return "", false
		}
		q := url.Values{}
		q.Set("security", "tls")
		if entry.SNI != "" {
			q.Set("sni", entry.SNI)
		}
		q.Set("type", "tcp")
		if entry.ALPN != "" {
			q.Set("alpn", entry.ALPN)
		}
		u := url.URL{
			Scheme:   "trojan",
			User:     url.User(entry.Secret),
			Host:     hostPort,
			RawQuery: q.Encode(),
			Fragment: fragment,
		}
		return u.String(), true
	case "anytls":
		if entry.Secret == "" {
			return "", false
		}
		q := url.Values{}
		if entry.SNI != "" {
			q.Set("sni", entry.SNI)
		}
		u := url.URL{
			Scheme:   "anytls",
			User:     url.User(entry.Secret),
			Host:     hostPort,
			RawQuery: q.Encode(),
			Fragment: fragment,
		}
		return u.String(), true
	case "ss":
		if entry.Method == "" || entry.Secret == "" {
			return "", false
		}
		auth := base64.StdEncoding.EncodeToString([]byte(entry.Method + ":" + entry.Secret))
		line := fmt.Sprintf("ss://%s@%s#%s", auth, hostPort, url.QueryEscape(fragment))
		return line, true
	default:
		return "", false
	}
}

func EncodeSingboxPayload(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	payload := strings.Join(lines, "\n")
	return base64.StdEncoding.EncodeToString([]byte(payload))
}
