package protocol

import (
	"fmt"

	"go-proxy/internal/crypto"
	"go-proxy/internal/store"
)

// FindExistingInbound returns the first inbound matching the given protocol type, or nil.
// It distinguishes Reality vs non-Reality variants of the same sing-box type.
func FindExistingInbound(s *store.Store, pt Type) *store.Inbound {
	spec := Specs()[pt]
	if spec.SingBoxType == "" {
		return nil
	}
	for i := range s.SingBox.Inbounds {
		ib := &s.SingBox.Inbounds[i]
		if ib.Type != spec.SingBoxType {
			continue
		}
		ibHasReality := ib.TLS != nil && ib.TLS.Reality != nil && ib.TLS.Reality.Enabled
		if ibHasReality == spec.UsesReality {
			return ib
		}
	}
	return nil
}

// AddUserToExisting adds a new user to an existing inbound, generating appropriate credentials.
func AddUserToExisting(s *store.Store, ib *store.Inbound, userName string) (*InstallResult, error) {
	for _, u := range ib.Users {
		if u.Name == userName {
			return nil, fmt.Errorf("用户 %q 已存在", userName)
		}
	}

	user := store.User{Name: userName}

	switch ib.Type {
	case "vless":
		uuid, err := crypto.GenerateUUID()
		if err != nil {
			return nil, err
		}
		user.UUID = uuid
		user.Flow = "xtls-rprx-vision"
		ib.Users = append(ib.Users, user)
		s.MarkDirty(store.FileSingBox)
		return &InstallResult{Tag: ib.Tag, Port: ib.ListenPort, Credential: uuid}, nil

	case "trojan", "anytls":
		password, err := crypto.GeneratePassword(16)
		if err != nil {
			return nil, err
		}
		user.Password = password
		ib.Users = append(ib.Users, user)
		s.MarkDirty(store.FileSingBox)
		return &InstallResult{Tag: ib.Tag, Port: ib.ListenPort, Credential: password}, nil

	case "tuic":
		uuid, err := crypto.GenerateUUID()
		if err != nil {
			return nil, err
		}
		password, err := crypto.GeneratePassword(16)
		if err != nil {
			return nil, err
		}
		user.UUID = uuid
		user.Password = password
		ib.Users = append(ib.Users, user)
		s.MarkDirty(store.FileSingBox)
		return &InstallResult{Tag: ib.Tag, Port: ib.ListenPort, Credential: uuid}, nil

	case "shadowsocks":
		method := ib.Method
		if method == "" {
			method = crypto.DefaultSSMethod
		}
		keySize := crypto.SSKeySize(method)
		userKey, err := crypto.GenerateSSKey(keySize)
		if err != nil {
			return nil, err
		}
		user.Password = userKey
		ib.Users = append(ib.Users, user)
		s.MarkDirty(store.FileSingBox)
		return &InstallResult{Tag: ib.Tag, Port: ib.ListenPort, Credential: userKey}, nil

	default:
		return nil, fmt.Errorf("unsupported inbound type for adding user: %s", ib.Type)
	}
}

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
	// TUIC parameters.
	CongestionControl string // e.g., "bbr", "cubic"
	// Snell parameters.
	SnellIPv6 bool
	SnellObfs string // "off", "http", "tls"
	SnellUDP  bool
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
		recordInstalledUserMeta(s, params.ProtoType, store.SnellTag, psk, params.UserName)
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

	recordInstalledUserMeta(s, params.ProtoType, tag, result.Credential, params.UserName)
	s.MarkDirty(store.FileSingBox)
	return &result, nil
}

func recordInstalledUserMeta(s *store.Store, pt Type, tag, credential, userName string) {
	if s == nil || tag == "" || credential == "" || userName == "" {
		return
	}
	if s.UserMeta == nil {
		s.UserMeta = store.NewUserManagement()
	}
	if s.UserMeta.Name == nil {
		s.UserMeta.Name = make(map[string]string)
	}

	metaProto := Specs()[pt].SingBoxType
	if metaProto == "" {
		metaProto = string(pt)
	}
	s.UserMeta.Name[store.UserKey(metaProto, tag, credential)] = userName
	s.MarkDirty(store.FileUserMeta)
}

func buildVLESSInbound(tag string, p InstallParams) (*store.Inbound, string, error) {
	uuid, err := crypto.GenerateUUID()
	if err != nil {
		return nil, "", err
	}
	tls := buildStandardTLS(p)
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
	return ib, uuid, nil
}

// buildStandardTLS creates a TLS config with certificate paths for non-Reality protocols.
func buildStandardTLS(p InstallParams) *store.TLSConfig {
	domain := p.Domain
	if domain == "" {
		domain = DetectTLSDomain()
	}
	tls := &store.TLSConfig{
		Enabled:    true,
		ServerName: domain,
	}
	certPath, keyPath := ResolveTLSCertPaths(domain)
	if certPath != "" {
		tls.CertificatePath = certPath
		tls.KeyPath = keyPath
	}
	return tls
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
	tls := buildStandardTLS(p)
	tls.ALPN = []string{"h3"}
	congestion := p.CongestionControl
	if congestion == "" {
		congestion = "bbr"
	}
	ib := &store.Inbound{
		Type:              "tuic",
		Tag:               tag,
		Listen:            "0.0.0.0",
		ListenPort:        p.Port,
		CongestionControl: congestion,
		Users: []store.User{
			{Name: p.UserName, UUID: uuid, Password: password},
		},
		TLS: tls,
	}
	return ib, uuid, nil
}

func buildTrojanInbound(tag string, p InstallParams, _ bool) (*store.Inbound, string, error) {
	password, err := crypto.GeneratePassword(16)
	if err != nil {
		return nil, "", err
	}
	tls := buildStandardTLS(p)
	tls.ALPN = []string{"h2", "http/1.1"}
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
	tls := buildStandardTLS(p)
	ib := &store.Inbound{
		Type:       "anytls",
		Tag:        tag,
		Listen:     "0.0.0.0",
		ListenPort: p.Port,
		Users: []store.User{
			{Name: p.UserName, Password: password},
		},
		TLS: tls,
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
	obfs := p.SnellObfs
	if obfs == "" {
		obfs = "off"
	}
	conf := &store.SnellConfig{
		Listen: fmt.Sprintf("0.0.0.0:%d", p.Port),
		PSK:    psk,
		IPv6:   p.SnellIPv6,
		Obfs:   obfs,
		UDP:    p.SnellUDP,
	}
	return conf, psk, nil
}
