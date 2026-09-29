package protocol

import (
	"fmt"
	"math/rand"
)

// Type represents a supported proxy protocol.
type Type string

const (
	VLESS        Type = "vless"
	VLESSReality Type = "vless-reality"
	TUIC         Type = "tuic"
	AnyTLS       Type = "anytls"
	Snell        Type = "snell"
	ShadowTLS    Type = "shadow-tls"
)

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
	AnyTLS: {
		Type: AnyTLS, DisplayName: "anytls", SingBoxType: "anytls",
		DedicatedPort: false, NeedsTLS: true,
	},
	Snell: {
		Type: Snell, DisplayName: "snell-v6", SingBoxType: "",
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
	case VLESS, VLESSReality, AnyTLS:
		return []int{443, 2053, 2083, 2087, 2096, 8443, 9443}
	case Snell:
		return []int{443, 1443, 8443, 10443}
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
