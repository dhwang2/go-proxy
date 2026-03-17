package protocol

import "fmt"

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
