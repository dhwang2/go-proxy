package protocol

import (
	"fmt"

	"go-proxy/internal/crypto"
	"go-proxy/internal/store"
)

// InstallParams holds the parameters needed to install a protocol.
type InstallParams struct {
	ProtoType Type
	Port      int
	UserName  string
	// TLS parameters (for non-Reality protocols).
	Domain string
	// Reality parameters.
	SNI string // Server Name Indication / decoy domain
	// Shadowsocks parameters.
	SSMethod string // e.g., "2022-blake3-aes-256-gcm"
}

// InstallResult holds the output of a protocol installation.
type InstallResult struct {
	Tag        string
	Port       int
	Credential string // UUID, password, or PSK depending on protocol
	PublicKey  string // Reality public key (if applicable)
}

// Install adds a new protocol inbound to the store.
func Install(s *store.Store, params InstallParams) (*InstallResult, error) {
	spec, ok := Specs()[params.ProtoType]
	if !ok {
		return nil, fmt.Errorf("unknown protocol type: %s", params.ProtoType)
	}

	if params.UserName == "" {
		params.UserName = "user"
	}

	tag := InboundTag(params.ProtoType, params.Port)

	// Check for duplicate tag.
	for _, ib := range s.SingBox.Inbounds {
		if ib.Tag == tag {
			return nil, fmt.Errorf("inbound tag %q already exists", tag)
		}
	}

	var result InstallResult
	result.Tag = tag
	result.Port = params.Port

	switch params.ProtoType {
	case VLESS:
		ib, cred, err := buildVLESSInbound(tag, params)
		if err != nil {
			return nil, err
		}
		s.SingBox.Inbounds = append(s.SingBox.Inbounds, *ib)
		result.Credential = cred

	case VLESSReality:
		ib, cred, pubKey, err := buildVLESSRealityInbound(tag, params)
		if err != nil {
			return nil, err
		}
		s.SingBox.Inbounds = append(s.SingBox.Inbounds, *ib)
		result.Credential = cred
		result.PublicKey = pubKey

	case TUIC:
		ib, cred, err := buildTUICInbound(tag, params)
		if err != nil {
			return nil, err
		}
		s.SingBox.Inbounds = append(s.SingBox.Inbounds, *ib)
		result.Credential = cred

	case Trojan:
		ib, cred, err := buildTrojanInbound(tag, params, false)
		if err != nil {
			return nil, err
		}
		s.SingBox.Inbounds = append(s.SingBox.Inbounds, *ib)
		result.Credential = cred

	case TrojanReality:
		ib, cred, pubKey, err := buildTrojanRealityInbound(tag, params)
		if err != nil {
			return nil, err
		}
		s.SingBox.Inbounds = append(s.SingBox.Inbounds, *ib)
		result.Credential = cred
		result.PublicKey = pubKey

	case AnyTLS:
		ib, cred, err := buildAnyTLSInbound(tag, params)
		if err != nil {
			return nil, err
		}
		s.SingBox.Inbounds = append(s.SingBox.Inbounds, *ib)
		result.Credential = cred

	case Shadowsocks:
		ib, cred, err := buildSSInbound(tag, params)
		if err != nil {
			return nil, err
		}
		s.SingBox.Inbounds = append(s.SingBox.Inbounds, *ib)
		result.Credential = cred

	case Snell:
		conf, psk, err := buildSnellConfig(params)
		if err != nil {
			return nil, err
		}
		s.SnellConf = conf
		s.MarkDirty(store.FileSnellConf)
		result.Credential = psk
		return &result, nil

	case ShadowTLS:
		// ShadowTLS is managed via external binary + systemd unit.
		// The actual setup is handled by the service package.
		return nil, fmt.Errorf("shadow-tls installation requires service package (Phase 3)")

	default:
		return nil, fmt.Errorf("protocol %s not implemented", spec.DisplayName)
	}

	s.MarkDirty(store.FileSingBox)
	return &result, nil
}

func buildVLESSInbound(tag string, p InstallParams) (*store.Inbound, string, error) {
	uuid, err := crypto.GenerateUUID()
	if err != nil {
		return nil, "", err
	}
	ib := &store.Inbound{
		Type:       "vless",
		Tag:        tag,
		Listen:     "0.0.0.0",
		ListenPort: p.Port,
		Users: []store.User{
			{Name: p.UserName, UUID: uuid, Flow: "xtls-rprx-vision"},
		},
		TLS: &store.TLSConfig{
			Enabled:    true,
			ServerName: p.Domain,
		},
	}
	return ib, uuid, nil
}

// buildRealityTLS generates a Reality-enabled TLS config with fresh keypair and short ID.
func buildRealityTLS(p InstallParams) (*store.TLSConfig, string, error) {
	kp, err := crypto.GenerateRealityKeypair()
	if err != nil {
		return nil, "", err
	}
	shortID, err := crypto.GenerateShortID()
	if err != nil {
		return nil, "", err
	}
	sni := p.SNI
	if sni == "" {
		sni = p.Domain
	}
	return &store.TLSConfig{
		Enabled:    true,
		ServerName: sni,
		Reality: &store.RealityConfig{
			Enabled: true,
			Handshake: &store.RealityHandshake{
				Server:     sni,
				ServerPort: 443,
			},
			PrivateKey: kp.PrivateKey,
			ShortID:    []string{shortID},
		},
	}, kp.PublicKey, nil
}

func buildVLESSRealityInbound(tag string, p InstallParams) (*store.Inbound, string, string, error) {
	uuid, err := crypto.GenerateUUID()
	if err != nil {
		return nil, "", "", err
	}
	tls, pubKey, err := buildRealityTLS(p)
	if err != nil {
		return nil, "", "", err
	}
	ib := &store.Inbound{
		Type:       "vless",
		Tag:        tag,
		Listen:     "0.0.0.0",
		ListenPort: p.Port,
		Users: []store.User{
			{Name: p.UserName, UUID: uuid, Flow: "xtls-rprx-vision"},
		},
		TLS: tls,
	}
	return ib, uuid, pubKey, nil
}

func buildTUICInbound(tag string, p InstallParams) (*store.Inbound, string, error) {
	uuid, err := crypto.GenerateUUID()
	if err != nil {
		return nil, "", err
	}
	password, err := crypto.GeneratePassword(16)
	if err != nil {
		return nil, "", err
	}
	ib := &store.Inbound{
		Type:              "tuic",
		Tag:               tag,
		Listen:            "0.0.0.0",
		ListenPort:        p.Port,
		CongestionControl: "bbr",
		Users: []store.User{
			{Name: p.UserName, UUID: uuid, Password: password},
		},
		TLS: &store.TLSConfig{
			Enabled:    true,
			ServerName: p.Domain,
			ALPN:       []string{"h3"},
		},
	}
	return ib, uuid, nil
}

func buildTrojanInbound(tag string, p InstallParams, _ bool) (*store.Inbound, string, error) {
	password, err := crypto.GeneratePassword(16)
	if err != nil {
		return nil, "", err
	}
	ib := &store.Inbound{
		Type:       "trojan",
		Tag:        tag,
		Listen:     "0.0.0.0",
		ListenPort: p.Port,
		Users: []store.User{
			{Name: p.UserName, Password: password},
		},
		TLS: &store.TLSConfig{
			Enabled:    true,
			ServerName: p.Domain,
			ALPN:       []string{"h2", "http/1.1"},
		},
	}
	return ib, password, nil
}

func buildTrojanRealityInbound(tag string, p InstallParams) (*store.Inbound, string, string, error) {
	password, err := crypto.GeneratePassword(16)
	if err != nil {
		return nil, "", "", err
	}
	tls, pubKey, err := buildRealityTLS(p)
	if err != nil {
		return nil, "", "", err
	}
	ib := &store.Inbound{
		Type:       "trojan",
		Tag:        tag,
		Listen:     "0.0.0.0",
		ListenPort: p.Port,
		Users: []store.User{
			{Name: p.UserName, Password: password},
		},
		TLS: tls,
	}
	return ib, password, pubKey, nil
}

func buildAnyTLSInbound(tag string, p InstallParams) (*store.Inbound, string, error) {
	password, err := crypto.GeneratePassword(16)
	if err != nil {
		return nil, "", err
	}
	ib := &store.Inbound{
		Type:       "anytls",
		Tag:        tag,
		Listen:     "0.0.0.0",
		ListenPort: p.Port,
		Users: []store.User{
			{Name: p.UserName, Password: password},
		},
		TLS: &store.TLSConfig{
			Enabled:    true,
			ServerName: p.Domain,
		},
	}
	return ib, password, nil
}

func buildSSInbound(tag string, p InstallParams) (*store.Inbound, string, error) {
	method := p.SSMethod
	if method == "" {
		method = crypto.DefaultSSMethod
	}
	keySize := crypto.SSKeySize(method)
	serverKey, err := crypto.GenerateSSKey(keySize)
	if err != nil {
		return nil, "", err
	}
	userKey, err := crypto.GenerateSSKey(keySize)
	if err != nil {
		return nil, "", err
	}
	ib := &store.Inbound{
		Type:       "shadowsocks",
		Tag:        tag,
		Listen:     "0.0.0.0",
		ListenPort: p.Port,
		Method:     method,
		Password:   serverKey,
		Users: []store.User{
			{Name: p.UserName, Password: userKey},
		},
	}
	return ib, userKey, nil
}

func buildSnellConfig(p InstallParams) (*store.SnellConfig, string, error) {
	psk, err := crypto.GeneratePassword(32)
	if err != nil {
		return nil, "", err
	}
	conf := &store.SnellConfig{
		Listen: fmt.Sprintf("0.0.0.0:%d", p.Port),
		PSK:    psk,
	}
	return conf, psk, nil
}
