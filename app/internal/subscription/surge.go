package subscription

import (
	"fmt"
	"strings"
)

func RenderSurge(entry protocolEntry, targets []Target, nodeName string) []string {
	if len(targets) == 0 {
		return nil
	}
	protoLabel, extra, ok := surgeLineParts(entry)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(targets))
	for _, t := range targets {
		suffix := surgeTargetSuffix(t.Family)
		name := fmt.Sprintf("%s-%s-%s-%s", nodeName, entry.Protocol, suffix, entry.Group)
		out = append(out, fmt.Sprintf("%s = %s, %s, %d%s", name, protoLabel, t.Host, entry.Port, extra))
	}
	return out
}

func surgeTargetSuffix(family string) string {
	switch family {
	case "ipv6":
		return "v6"
	case "domain":
		return "domain"
	default:
		return "v4"
	}
}

func surgeLineParts(entry protocolEntry) (string, string, bool) {
	switch entry.Protocol {
	case "tuic":
		if entry.Secret == "" || entry.UserID == "" {
			return "", "", false
		}
		extra := fmt.Sprintf(", password=%s, uuid=%s, alpn=h3", entry.Secret, strings.ToUpper(entry.UserID))
		if entry.SNI != "" {
			extra += ", sni=" + entry.SNI
		}
		extra += ", skip-cert-verify=false"
		return "tuic-v5", extra, true
	case "trojan":
		if entry.Secret == "" {
			return "", "", false
		}
		extra := ", password=" + entry.Secret
		if entry.SNI != "" {
			extra += ", sni=" + entry.SNI
		}
		if alpn := firstALPN(entry.ALPN); alpn != "" {
			extra += ", alpn=" + alpn
		}
		extra += ", skip-cert-verify=false"
		return "trojan", extra, true
	case "anytls":
		if entry.Secret == "" {
			return "", "", false
		}
		extra := ", password=" + entry.Secret
		if entry.SNI != "" {
			extra += ", sni=" + entry.SNI
		}
		extra += ", skip-cert-verify=false"
		return "anytls", extra, true
	case "ss":
		if entry.Method == "" || entry.Secret == "" {
			return "", "", false
		}
		escaped := strings.ReplaceAll(entry.Secret, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
		extra := fmt.Sprintf(", encrypt-method=%s, password=\"%s\"", entry.Method, escaped)
		return "ss", extra, true
	case "snell":
		if entry.Secret == "" {
			return "", "", false
		}
		extra := fmt.Sprintf(", psk=%s, version=5", entry.Secret)
		return "snell", extra, true
	default:
		return "", "", false
	}
}

func firstALPN(raw string) string {
	for _, item := range strings.Split(raw, ",") {
		s := strings.TrimSpace(item)
		if s != "" {
			return s
		}
	}
	return ""
}
