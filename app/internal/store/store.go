package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/dhwang2/go-proxy/pkg/sysutil"
)

// DefaultWorkDir is the default runtime root directory for go-proxy.
const DefaultWorkDir = "/etc/go-proxy"

type Paths struct {
	ConfPath     string
	UserMetaPath string
	SnellPath    string
}

func DefaultPaths(workDir string) Paths {
	if workDir == "" {
		workDir = DefaultWorkDir
	}
	confDir := filepath.Join(workDir, "conf")
	conf := filepath.Join(confDir, "config.json")
	if _, err := os.Stat(conf); err != nil {
		legacy := filepath.Join(confDir, "sing-box.json")
		if _, errLegacy := os.Stat(legacy); errLegacy == nil {
			conf = legacy
		} else if matches, errGlob := filepath.Glob(filepath.Join(confDir, "*.json")); errGlob == nil && len(matches) > 0 {
			conf = matches[0]
		}
	}
	return Paths{
		ConfPath:     conf,
		UserMetaPath: filepath.Join(workDir, "user-management.json"),
		SnellPath:    filepath.Join(workDir, "snell-v5.conf"),
	}
}

type Store struct {
	Config    *SingboxConfig
	UserMeta  *UserMeta
	SnellConf *SnellConfig
	Platform  sysutil.PlatformInfo

	confPath     string
	userMetaPath string
	snellPath    string

	configDirty   bool
	userMetaDirty bool
	snellDirty    bool

	mu sync.Mutex
}

func Load(confPath, userMetaPath, snellPath string) (*Store, error) {
	st := &Store{
		confPath:     confPath,
		userMetaPath: userMetaPath,
		snellPath:    snellPath,
		Config:       &SingboxConfig{},
		UserMeta:     DefaultUserMeta(),
		Platform:     sysutil.DetectPlatform(),
	}
	if err := st.loadConfig(); err != nil {
		return nil, err
	}
	if err := st.loadUserMeta(); err != nil {
		return nil, err
	}
	if err := st.loadSnell(); err != nil {
		return nil, err
	}
	return st, nil
}

func (s *Store) ConfPath() string {
	return s.confPath
}

func (s *Store) UserMetaPath() string {
	return s.userMetaPath
}

func (s *Store) SnellPath() string {
	return s.snellPath
}

func (s *Store) loadConfig() error {
	b, err := os.ReadFile(s.confPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.Config = &SingboxConfig{}
			return nil
		}
		return fmt.Errorf("read config: %w", err)
	}
	if len(b) == 0 {
		s.Config = &SingboxConfig{}
		return nil
	}
	var cfg SingboxConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	s.Config = &cfg
	return nil
}

func (s *Store) loadUserMeta() error {
	b, err := os.ReadFile(s.userMetaPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.UserMeta = DefaultUserMeta()
			return nil
		}
		return fmt.Errorf("read user meta: %w", err)
	}
	if len(b) == 0 {
		s.UserMeta = DefaultUserMeta()
		return nil
	}
	var meta UserMeta
	if err := json.Unmarshal(b, &meta); err != nil {
		return fmt.Errorf("decode user meta: %w", err)
	}
	meta.EnsureDefaults()
	s.UserMeta = &meta
	return nil
}

func (s *Store) loadSnell() error {
	b, err := os.ReadFile(s.snellPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.SnellConf = &SnellConfig{Values: map[string]string{}}
			return nil
		}
		return fmt.Errorf("read snell config: %w", err)
	}
	s.SnellConf = ParseSnellConfig(b)
	return nil
}

func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.configDirty {
		payload, err := json.MarshalIndent(s.Config, "", "  ")
		if err != nil {
			return fmt.Errorf("encode config: %w", err)
		}
		payload = append(payload, '\n')
		if err := atomicWrite(s.confPath, payload, 0o644); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
		s.configDirty = false
	}

	if s.userMetaDirty {
		payload, err := json.MarshalIndent(s.UserMeta, "", "  ")
		if err != nil {
			return fmt.Errorf("encode user meta: %w", err)
		}
		payload = append(payload, '\n')
		if err := atomicWrite(s.userMetaPath, payload, 0o644); err != nil {
			return fmt.Errorf("write user meta: %w", err)
		}
		s.userMetaDirty = false
	}

	if s.snellDirty {
		payload := s.SnellConf.MarshalText()
		if err := atomicWrite(s.snellPath, payload, 0o644); err != nil {
			return fmt.Errorf("write snell config: %w", err)
		}
		s.snellDirty = false
	}

	return nil
}

func (s *Store) MarkConfigDirty() {
	s.configDirty = true
}

func (s *Store) MarkUserMetaDirty() {
	s.userMetaDirty = true
}

func (s *Store) MarkSnellDirty() {
	s.snellDirty = true
}

func (s *Store) ConfigDirty() bool {
	return s.configDirty
}

func (s *Store) UserMetaDirty() bool {
	return s.userMetaDirty
}

func (s *Store) SnellDirty() bool {
	return s.snellDirty
}
