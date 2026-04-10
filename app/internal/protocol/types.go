package protocol

import (
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"go-proxy/internal/config"
)

// Type represents a supported proxy protocol.
type Type string

const (
	VLESS         Type = "vless"
	VLESSReality  Type = "vless-reality"
	TUIC          Type = "tuic"
	Trojan        Type = "trojan"
	TrojanReality Type = "trojan-reality"
	AnyTLS        Type = "anytls"
	Shadowsocks   Type = "shadowsocks"
	Snell         Type = "snell"
	ShadowTLS     Type = "shadow-tls"
)

// AllTypes returns all supported protocol types.
func AllTypes() []Type {
	return []Type{
		VLESS, VLESSReality, TUIC, Trojan, TrojanReality,
		AnyTLS, Shadowsocks, Snell, ShadowTLS,
	}
}

// InstallableTypes returns the 6 user-facing protocols in shell-proxy menu order.
// Reality/ShadowTLS variants are sub-options during the install flow, not top-level choices.
func InstallableTypes() []Type {
	return []Type{Shadowsocks, VLESS, TUIC, Trojan, AnyTLS, Snell}
}

// Spec describes protocol characteristics.
type Spec struct {
	Type          Type
	DisplayName   string
	SingBoxType   string // sing-box inbound type field
	DedicatedPort bool   // needs its own port (vs shared 443)
	NeedsTLS      bool
	UsesReality   bool
	ExternalBin   string // empty if managed by sing-box
}

// specs is the package-level protocol specification map (read-only after init).
var specs = map[Type]Spec{
	VLESS: {
		Type: VLESS, DisplayName: "vless", SingBoxType: "vless",
		DedicatedPort: false, NeedsTLS: true,
	},
	VLESSReality: {
		Type: VLESSReality, DisplayName: "vless + reality", SingBoxType: "vless",
		DedicatedPort: true, NeedsTLS: true, UsesReality: true,
	},
	TUIC: {
		Type: TUIC, DisplayName: "tuic", SingBoxType: "tuic",
		DedicatedPort: true, NeedsTLS: true,
	},
	Trojan: {
		Type: Trojan, DisplayName: "trojan", SingBoxType: "trojan",
		DedicatedPort: false, NeedsTLS: true,
	},
	TrojanReality: {
		Type: TrojanReality, DisplayName: "trojan + reality", SingBoxType: "trojan",
		DedicatedPort: true, NeedsTLS: true, UsesReality: true,
	},
	AnyTLS: {
		Type: AnyTLS, DisplayName: "anytls", SingBoxType: "anytls",
		DedicatedPort: false, NeedsTLS: true,
	},
	Shadowsocks: {
		Type: Shadowsocks, DisplayName: "ss", SingBoxType: "shadowsocks",
		DedicatedPort: true, NeedsTLS: false,
	},
	Snell: {
		Type: Snell, DisplayName: "snell-v5", SingBoxType: "",
		DedicatedPort: true, NeedsTLS: false, ExternalBin: "snell-server",
	},
	ShadowTLS: {
		Type: ShadowTLS, DisplayName: "ShadowTLS v3", SingBoxType: "",
		DedicatedPort: true, NeedsTLS: false, ExternalBin: "shadow-tls",
	},
}

// Specs returns the specification for each protocol type.
func Specs() map[Type]Spec {
	return specs
}

// InboundTag generates the canonical inbound tag for a protocol and port.
func InboundTag(protoType Type, port int) string {
	spec := Specs()[protoType]
	name := string(spec.SingBoxType)
	if name == "" {
		name = string(protoType)
	}
	if spec.UsesReality {
		name += "_reality"
	}
	return fmt.Sprintf("%s_%d", name, port)
}

// CommonPorts returns the preferred port candidates for a protocol type.
func CommonPorts(pt Type) []int {
	switch pt {
	case Trojan, VLESS, AnyTLS:
		return []int{443, 2053, 2083, 2087, 2096, 8443, 9443}
	case Snell:
		return []int{443, 1443, 8443, 10443}
	case Shadowsocks:
		return []int{443, 8388, 8443, 9443}
	case ShadowTLS:
		return []int{8443, 443, 9443, 10443}
	case TUIC:
		return nil // uses random high port
	default:
		return nil
	}
}

// DefaultPort picks an available default port for a protocol.
// It tries common ports first, then falls back to a random port in 20000-29999.
func DefaultPort(pt Type, usedPorts map[int]bool) int {
	candidates := CommonPorts(pt)
	for _, p := range candidates {
		if !usedPorts[p] {
			return p
		}
	}
	// Random high port in 20000-29999.
	for i := 0; i < 100; i++ {
		p := 20000 + rand.Intn(10000)
		if !usedPorts[p] {
			return p
		}
	}
	return 20000 + rand.Intn(10000)
}

// CollectUsedPorts collects all ports currently in use from inbound port list.
func CollectUsedPorts(ports []int) map[int]bool {
	used := make(map[int]bool)
	for _, p := range ports {
		if p > 0 {
			used[p] = true
		}
	}
	return used
}

// caddyCertIssuerDirs returns the issuer subdirectories under the caddy certificates directory.
func caddyCertIssuerDirs() []string {
	caddyCertDir := config.CaddyCertDir
	entries, err := os.ReadDir(caddyCertDir)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, filepath.Join(caddyCertDir, e.Name()))
		}
	}
	return dirs
}

// DetectTLSDomain reads the domain from /etc/go-proxy/.domain or detects it from
// caddy certificate directory.
func DetectTLSDomain() string {
	domainFile := filepath.Join(config.WorkDir, ".domain")
	if data, err := os.ReadFile(domainFile); err == nil {
		d := strings.TrimSpace(string(data))
		if d != "" {
			return d
		}
	}

	for _, issuerDir := range caddyCertIssuerDirs() {
		domain := ""
		_ = filepath.WalkDir(issuerDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d == nil || !d.IsDir() {
				return nil
			}
			name := d.Name()
			if name == "." || !strings.Contains(name, ".") {
				return nil
			}
			if _, key := resolveTLSCertPair(path, name); key != "" {
				domain = name
				return fs.SkipAll
			}
			return nil
		})
		if domain != "" {
			return domain
		}
	}
	return ""
}

// ResolveTLSCertPaths returns certificate and key file paths for a domain.
func ResolveTLSCertPaths(domain string) (certPath, keyPath string) {
	if domain == "" {
		return "", ""
	}
	for _, issuerDir := range caddyCertIssuerDirs() {
		cert, key := resolveTLSCertPair(issuerDir, domain)
		if cert != "" && key != "" {
			return cert, key
		}
	}
	return "", ""
}

func resolveTLSCertPair(rootDir, domain string) (string, string) {
	var certPath string
	var keyPath string
	_ = filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if filepath.Base(path) != domain+".crt" || filepath.Base(filepath.Dir(path)) != domain {
			return nil
		}
		keyCandidate := strings.TrimSuffix(path, ".crt") + ".key"
		if _, err := os.Stat(keyCandidate); err != nil {
			return nil
		}
		certPath = path
		keyPath = keyCandidate
		return fs.SkipAll
	})
	return certPath, keyPath
}
