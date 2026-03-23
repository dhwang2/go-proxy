package store

import (
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
	SnellConf    *SnellConfig // nil if snell is not installed

	dirty   map[string]bool
	applied bool // true after Apply() writes to disk — signals that Reload() should refresh
}

// File keys for MarkDirty/Apply.
const (
	FileSingBox      = "sing-box.json"
	FileUserMeta     = "user-management.json"
	FileUserRoutes   = "user-route-rules.json"
	FileUserTemplate = "user-route-templates.json"
	FileSnellConf    = "snell-v5.conf"
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

	// user-route-templates.json
	ut, err := loadJSON[UserRouteTemplates](config.UserTemplateFile)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", FileUserTemplate, err)
	}
	s.UserTemplate = ut
	if s.UserTemplate.Templates == nil {
		s.UserTemplate.Templates = make(map[string][]TemplateRule)
	}

	// snell-v5.conf (optional)
	if data, err := os.ReadFile(config.SnellConfigFile); err == nil {
		sc, err := ParseSnellConfig(string(data))
		if err == nil {
			s.SnellConf = sc
		}
	}

	return s, nil
}

// Reload re-reads all configuration files from disk into this store.
// Only performs I/O if the store was modified since last load (via Apply).
func (s *Store) Reload() error {
	if !s.applied {
		return nil
	}
	fresh, err := Load()
	if err != nil {
		return err
	}
	*s = *fresh
	return nil
}

// MarkDirty flags a config file for saving on the next Apply() call.
func (s *Store) MarkDirty(file string) {
	s.dirty[file] = true
}

// IsDirty returns whether any files are flagged for saving.
func (s *Store) IsDirty() bool {
	return len(s.dirty) > 0
}

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

// Apply saves dirty files, validates sing-box config, and returns.
// Service restart is handled by the caller (service package).
func (s *Store) Apply() error {
	if !s.IsDirty() {
		return nil
	}

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
		for file := range backups {
			fileutil.RestoreBackup(s.filePath(file))
		}
		return err
	}

	// Validate sing-box config if it was modified.
	if _, wasDirty := backups[FileSingBox]; wasDirty {
		if err := s.validateSingBox(); err != nil {
			// Restore backups on validation failure.
			for file := range backups {
				fileutil.RestoreBackup(s.filePath(file))
			}
			// Re-load the restored config.
			if restored, loadErr := Load(); loadErr == nil {
				*s = *restored
			}
			return fmt.Errorf("sing-box validation failed: %w", err)
		}
	}

	// Clean up backups.
	for file := range backups {
		fileutil.CleanBackup(s.filePath(file))
	}
	s.applied = true
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
	case FileSnellConf:
		return config.SnellConfigFile
	default:
		return ""
	}
}

func (s *Store) validateSingBox() error {
	bin := config.SingBoxBin
	if _, err := os.Stat(bin); os.IsNotExist(err) {
		// sing-box binary not installed yet; skip validation.
		return nil
	}
	cmd := exec.Command(bin, "check", "-c", config.SingBoxConfig)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
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
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	data = append(data, '\n')
	return fileutil.AtomicWrite(path, data)
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
