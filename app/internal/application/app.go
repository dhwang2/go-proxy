package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"

	"go-proxy/internal/config"
	"go-proxy/internal/network"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
	"go-proxy/pkg/fileutil"
)

type Result struct {
	Changed bool   `json:"changed"`
	Data    any    `json:"data,omitempty"`
	Raw     []byte `json:"-"`
	Silent  bool   `json:"-"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Stage   string `json:"stage,omitempty"`
	Changed bool   `json:"-"`
	Data    any    `json:"-"`
}

func (e *Error) Error() string { return e.Message }

func Invalid(message string) error { return &Error{Code: "invalid_argument", Message: message} }

type App struct {
	LockDir     string
	RequireRoot bool
	Progress    func(string)
}

func New(progress func(string)) *App {
	if progress == nil {
		progress = func(string) {}
	}
	return &App{LockDir: config.LockDir, RequireRoot: true, Progress: progress}
}

type Snapshot struct {
	Store     *store.Store
	Bindings  []service.ShadowTLSBinding
	stamp     string
	committed bool
}

func (a *App) lock(ctx context.Context, name string, exclusive, create bool) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	flags := os.O_RDONLY
	if create {
		flags = os.O_CREATE | os.O_RDWR
	}
	f, err := os.OpenFile(filepath.Join(a.LockDir, name), flags, 0600)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &Error{Code: "not_initialized", Message: "run gproxy init first"}
		}
		return nil, err
	}
	mode := unix.LOCK_SH | unix.LOCK_NB
	if exclusive {
		mode = unix.LOCK_EX | unix.LOCK_NB
	}
	if err := unix.Flock(int(f.Fd()), mode); err != nil {
		f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, &Error{Code: "busy", Message: "another operation is using the configuration"}
		}
		return nil, err
	}
	return func() { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN); _ = f.Close() }, nil
}

func (a *App) Operation(ctx context.Context, fn func() (Result, error)) (Result, error) {
	if a.RequireRoot && os.Geteuid() != 0 {
		return Result{}, &Error{Code: "permission_denied", Message: "this operation requires root"}
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(a.LockDir, 0755); err != nil {
		return Result{}, err
	}
	unlock, err := a.lock(ctx, ".operation.lock", true, true)
	if err != nil {
		return Result{}, err
	}
	defer unlock()
	f, err := os.OpenFile(filepath.Join(a.LockDir, ".state.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return Result{}, err
	}
	if err := f.Close(); err != nil {
		return Result{}, err
	}
	return fn()
}

func (a *App) State(ctx context.Context, fn func() error) error {
	unlock, err := a.lock(ctx, ".state.lock", true, false)
	if err != nil {
		return err
	}
	defer unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn()
}

func (a *App) Snapshot(ctx context.Context) (*Snapshot, error) {
	unlock, err := a.lock(ctx, ".state.lock", false, false)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if _, err := os.Stat(config.SingBoxConfig); err != nil {
		if os.IsNotExist(err) {
			return nil, &Error{Code: "not_initialized", Message: "run gproxy init first"}
		}
		return nil, err
	}
	before, err := fingerprint(ctx)
	if err != nil {
		return nil, err
	}
	s, err := store.Load()
	if err != nil {
		return nil, err
	}
	bindings, err := service.ListShadowTLSBindings(s)
	if err != nil {
		return nil, err
	}
	stamp, err := fingerprint(ctx)
	if err != nil {
		return nil, err
	}
	if before != stamp {
		return nil, &Error{Code: "conflict", Message: "configuration changed while reading; retry"}
	}
	return &Snapshot{Store: s, Bindings: bindings, stamp: stamp}, nil
}

func ObservationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	budget := 2 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		budget = remaining - 500*time.Millisecond
		if budget <= 0 {
			budget = remaining
		}
	}
	return context.WithTimeout(ctx, budget)
}

func fingerprint(ctx context.Context) (string, error) {
	h := sha256.New()
	for _, path := range []string{config.SingBoxConfig, config.UserMetaFile, config.UserRouteFile, config.UserTemplateFile, config.FirewallConfigFile, config.SnellConfigFile} {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		fmt.Fprintln(h, path)
		f, err := os.Open(path)
		if os.IsNotExist(err) {
			fmt.Fprintln(h, "missing")
			continue
		}
		if err != nil {
			return "", err
		}
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			return "", err
		}
		fmt.Fprintln(h)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (a *App) Commit(ctx context.Context, snapshot *Snapshot) error {
	if !snapshot.Store.IsDirty() {
		return nil
	}
	if err := snapshot.Store.ValidatePending(ctx); err != nil {
		return &Error{Code: "validation_failed", Stage: "validate", Message: err.Error()}
	}
	return a.State(ctx, func() error {
		current, err := fingerprint(ctx)
		if err != nil {
			return err
		}
		if current != snapshot.stamp {
			return &Error{Code: "conflict", Stage: "commit", Message: "configuration changed during preparation; retry the operation"}
		}
		pending, err := a.pendingServices()
		if err != nil {
			return err
		}
		if snapshot.Store.IsDirtyFile(store.FileSingBox) {
			pending[service.SingBox] = true
		}
		if snapshot.Store.IsDirtyFile(store.FileSnellConf) {
			pending[service.Snell] = true
		}
		if err := a.savePending(pending); err != nil {
			return err
		}
		if err := snapshot.Store.ApplyValidated(); err != nil {
			return &Error{Code: "save_failed", Stage: "commit", Message: err.Error(), Changed: true, Data: map[string]any{"pending": []string{"inspect configuration", "retry"}}}
		}
		snapshot.committed = true
		snapshot.stamp, err = fingerprint(ctx)
		if err != nil {
			return &Error{Code: "save_failed", Stage: "commit", Message: err.Error(), Changed: true}
		}
		return nil
	})
}

func (a *App) Activate(ctx context.Context, snapshot *Snapshot, names ...service.Name) error {
	pending, err := a.pendingServices()
	if err != nil {
		return err
	}
	seen := make(map[service.Name]bool)
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		configured := true
		if name == service.Snell {
			configured = snapshot.Store.SnellConf != nil
		}
		if name == service.SingBox {
			configured = len(snapshot.Store.SingBox.Inbounds) > 0
		}
		var err error
		if !configured {
			if !service.IsInstalled(ctx, name) {
				delete(pending, name)
				if err := a.State(ctx, func() error { return a.savePending(pending) }); err != nil {
					return err
				}
				continue
			}
			err = service.Stop(ctx, name)
			if err == nil {
				err = service.Disable(ctx, name)
			}
		} else {
			if service.IsStopped(name) {
				return &Error{Code: "service_stopped", Stage: "activate", Message: fmt.Sprintf("%s is intentionally stopped; use gproxy start %s", name, name), Changed: snapshot.committed, Data: map[string]any{"pending": []string{string(name)}}}
			}
			state, statusErr := service.GetStatus(ctx, name)
			if statusErr != nil {
				err = statusErr
			} else if pending[name] || state == nil || !state.Running {
				err = service.Enable(ctx, name)
				if err == nil {
					err = service.Restart(ctx, name)
				}
			}
		}
		if err != nil {
			return &Error{Code: "service_activation_failed", Stage: "activate", Message: fmt.Sprintf("activate %s: %v", name, err), Changed: true, Data: map[string]any{"applied": []string{"configuration"}, "pending": []string{string(name)}}}
		}
		if configured {
			if err := service.WaitReady(ctx, name); err != nil {
				return &Error{Code: "service_not_ready", Stage: "verify", Message: err.Error(), Changed: true, Data: map[string]any{"pending": []string{string(name)}}}
			}
			if err := verifyListeners(ctx, snapshot, name); err != nil {
				return &Error{Code: "listener_not_ready", Stage: "verify", Message: err.Error(), Changed: true, Data: map[string]any{"pending": []string{string(name)}}}
			}
		}
		delete(pending, name)
		if err := a.State(ctx, func() error { return a.savePending(pending) }); err != nil {
			return &Error{Code: "activation_record_failed", Stage: "activate", Message: err.Error(), Changed: true}
		}
	}
	managed, err := network.FirewallManaged(ctx)
	if err != nil {
		return &Error{Code: "firewall_failed", Stage: "firewall", Message: err.Error(), Changed: snapshot.committed}
	}
	if managed {
		if err := network.ApplyFirewallConvergence(ctx, snapshot.Store); err != nil {
			return &Error{Code: "firewall_failed", Stage: "firewall", Message: err.Error(), Changed: true, Data: map[string]any{"applied": []string{"configuration"}, "pending": []string{"firewall"}}}
		}
	}
	return nil
}

func (a *App) pendingServices() (map[service.Name]bool, error) {
	pending := make(map[service.Name]bool)
	data, err := os.ReadFile(filepath.Join(a.LockDir, "pending-services.json"))
	if os.IsNotExist(err) {
		return pending, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &pending); err != nil {
		return nil, fmt.Errorf("read pending activation state: %w", err)
	}
	return pending, nil
}

func (a *App) savePending(pending map[service.Name]bool) error {
	data, err := json.Marshal(pending)
	if err != nil {
		return err
	}
	return fileutil.AtomicWrite(filepath.Join(a.LockDir, "pending-services.json"), data)
}
