package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"go-proxy/internal/config"
	"go-proxy/pkg/fileutil"
)

// Store holds all configuration state in memory.
type Store struct {
	SingBox      *SingBoxConfig
	UserMeta     *UserManagement
	UserRoutes   []UserRouteRule
	UserTemplate *UserRouteTemplates
	Firewall     *FirewallConfig
	SnellConf    *SnellConfig // nil if snell is not installed

	dirty map[string]bool
}

// File keys for MarkDirty/Apply.
const (
	FileSingBox      = "sing-box.json"
	FileUserMeta     = "user-management.json"
	FileUserRoutes   = "user-route-rules.json"
	FileUserTemplate = "user-route-templates.json"
	FileFirewall     = "firewall-ports.json"
	FileSnellConf    = "snell-v6.conf"
)

// Load reads all configuration files from disk into memory.
// Missing files are initialized with defaults.
func Load() (*Store, error) {
	s := &Store{
		dirty: make(map[string]bool),
	}

	// sing-box.json
	sb, err := loadJSON[SingBoxConfig](config.SingBoxConfig)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", FileSingBox, err)
	}
	s.SingBox = sb
	s.SingBox.Normalize()

	// user-management.json
	um, err := loadJSON[UserManagement](config.UserMetaFile)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", FileUserMeta, err)
	}
	s.UserMeta = um
	if s.UserMeta.Schema == 0 {
		s.UserMeta.Schema = 3
	}
	ensureMetaMaps(s.UserMeta)

	// user-route-rules.json
	ur, err := loadJSONSlice[UserRouteRule](config.UserRouteFile)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", FileUserRoutes, err)
	}
	s.UserRoutes = ur
	for i := range s.UserRoutes {
		if tag := DirectOutbound(s.UserRoutes[i].Outbound); tag != s.UserRoutes[i].Outbound {
			s.UserRoutes[i].Outbound = tag
			s.MarkDirty(FileUserRoutes)
		}
	}

	// user-route-templates.json
	ut, err := loadJSON[UserRouteTemplates](config.UserTemplateFile)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", FileUserTemplate, err)
	}
	s.UserTemplate = ut
	if s.UserTemplate.Templates == nil {
		s.UserTemplate.Templates = make(map[string][]TemplateRule)
	}
	for _, rules := range s.UserTemplate.Templates {
		for i := range rules {
			if tag := DirectOutbound(rules[i].Outbound); tag != rules[i].Outbound {
				rules[i].Outbound = tag
				s.MarkDirty(FileUserTemplate)
			}
		}
	}

	fw, err := loadJSON[FirewallConfig](config.FirewallConfigFile)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", FileFirewall, err)
	}
	s.Firewall = fw
	s.Firewall.Normalize()

	// snell-v6.conf (optional)
	if data, err := os.ReadFile(config.SnellConfigFile); err == nil {
		sc, err := ParseSnellConfig(string(data))
		if err != nil {
			return nil, fmt.Errorf("parse snell configuration: %w", err)
		}
		s.SnellConf = sc
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read snell configuration: %w", err)
	}

	return s, nil
}

// MarkDirty flags a config file for saving on the next commit.
func (s *Store) MarkDirty(file string) {
	s.dirty[file] = true
}

// Settled reports whether a dirty file would be written back byte for byte as
// it already is on disk, and drops it from the dirty set when so. Recompiling a
// configuration that was already correct produces the same bytes; treating that
// as a change makes an operation claim it did something it did not.
//
// A file that cannot be read or rendered stays dirty: the answer is unknown,
// and writing it is the safe direction.
func (s *Store) Settled(file string) bool {
	if !s.dirty[file] {
		return true
	}
	rendered, err := s.render(file)
	if err != nil {
		return false
	}
	current, err := os.ReadFile(s.filePath(file))
	if err != nil || !bytes.Equal(current, rendered) {
		return false
	}
	delete(s.dirty, file)
	return true
}

// render produces exactly what saveFile would write, without writing it.
func (s *Store) render(file string) ([]byte, error) {
	switch file {
	case FileSingBox:
		s.SingBox.Normalize()
		return marshalJSON(s.SingBox)
	case FileUserMeta:
		return marshalJSON(s.UserMeta)
	case FileUserRoutes:
		return marshalJSON(s.UserRoutes)
	case FileUserTemplate:
		return marshalJSON(s.UserTemplate)
	case FileFirewall:
		if s.Firewall == nil {
			s.Firewall = &FirewallConfig{}
		}
		s.Firewall.Normalize()
		return marshalJSON(s.Firewall)
	case FileSnellConf:
		if s.SnellConf == nil {
			return nil, fmt.Errorf("no snell configuration to render")
		}
		return s.SnellConf.MarshalSnellConfig(), nil
	}
	return nil, fmt.Errorf("unknown file: %s", file)
}

// IsDirty returns whether any files are flagged for saving.
func (s *Store) IsDirty() bool {
	return len(s.dirty) > 0
}

func (s *Store) IsDirtyFile(file string) bool { return s.dirty[file] }

// Save atomically writes all dirty files to disk without validation or restart.
func (s *Store) Save() error {
	for file := range s.dirty {
		if err := s.saveFile(file); err != nil {
			return err
		}
	}
	s.dirty = make(map[string]bool)
	return nil
}

func (s *Store) ApplyValidated() error {

	// Backup dirty files.
	backups := make(map[string]string)
	for file := range s.dirty {
		path := s.filePath(file)
		bak, err := fileutil.Backup(path)
		if err != nil {
			return fmt.Errorf("backup %s: %w", file, err)
		}
		backups[file] = bak
	}

	// Save all dirty files.
	if err := s.Save(); err != nil {
		// Restore backups on failure.
		for file, backup := range backups {
			if backup == "" {
				os.Remove(s.filePath(file))
			} else {
				fileutil.RestoreBackup(s.filePath(file))
			}
		}
		return err
	}

	// Clean up backups.
	for file := range backups {
		fileutil.CleanBackup(s.filePath(file))
	}
	return nil
}

func (s *Store) saveFile(file string) error {
	switch file {
	case FileSingBox:
		s.SingBox.Normalize()
		return writeJSON(config.SingBoxConfig, s.SingBox)
	case FileUserMeta:
		return writeJSON(config.UserMetaFile, s.UserMeta)
	case FileUserRoutes:
		return writeJSON(config.UserRouteFile, s.UserRoutes)
	case FileUserTemplate:
		return writeJSON(config.UserTemplateFile, s.UserTemplate)
	case FileFirewall:
		if s.Firewall == nil {
			s.Firewall = &FirewallConfig{}
		}
		s.Firewall.Normalize()
		return writeJSON(config.FirewallConfigFile, s.Firewall)
	case FileSnellConf:
		if s.SnellConf == nil {
			if err := os.Remove(config.SnellConfigFile); err != nil && !os.IsNotExist(err) {
				return err
			}
			return nil
		}
		return fileutil.AtomicWrite(config.SnellConfigFile, s.SnellConf.MarshalSnellConfig())
	default:
		return fmt.Errorf("unknown file: %s", file)
	}
}

func (s *Store) filePath(file string) string {
	switch file {
	case FileSingBox:
		return config.SingBoxConfig
	case FileUserMeta:
		return config.UserMetaFile
	case FileUserRoutes:
		return config.UserRouteFile
	case FileUserTemplate:
		return config.UserTemplateFile
	case FileFirewall:
		return config.FirewallConfigFile
	case FileSnellConf:
		return config.SnellConfigFile
	default:
		return ""
	}
}

// Validate checks the current sing-box configuration without saving.
func (s *Store) Validate(ctx context.Context) error {
	s.SingBox.Normalize()
	f, err := os.CreateTemp("", "gproxy-validate-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := json.NewEncoder(f).Encode(s.SingBox); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return validateSingBox(ctx, f.Name())
}

func (s *Store) ValidatePending(ctx context.Context) error {
	if !s.dirty[FileSingBox] {
		return nil
	}
	return s.Validate(ctx)
}

func validateSingBox(ctx context.Context, path string) error {
	bin := config.SingBoxBin
	if _, err := os.Stat(bin); os.IsNotExist(err) {
		return fmt.Errorf("sing-box validator is not installed")
	}
	cmd := exec.CommandContext(ctx, bin, "check", "-c", path)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sing-box validation failed: %w", err)
	}
	return nil
}

// loadJSON reads a JSON file into a typed struct.
// Returns a zero-value struct if the file does not exist.
func loadJSON[T any](path string) (*T, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return new(T), nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return new(T), nil
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &v, nil
}

// loadJSONSlice reads a JSON array file into a typed slice.
func loadJSONSlice[T any](path string) ([]T, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var v []T
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return v, nil
}

func writeJSON(path string, v any) error {
	data, err := marshalJSON(v)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	return fileutil.AtomicWrite(path, data)
}

func marshalJSON(v any) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func ensureMetaMaps(um *UserManagement) {
	if um.Disabled == nil {
		um.Disabled = make(map[string]DisabledEntry)
	}
	if um.Expiry == nil {
		um.Expiry = make(map[string]string)
	}
	if um.Route == nil {
		um.Route = make(map[string][]string)
	}
	if um.Template == nil {
		um.Template = make(map[string]string)
	}
	if um.Name == nil {
		um.Name = make(map[string]string)
	}
	if um.Groups == nil {
		um.Groups = make(map[string][]string)
	}
}
