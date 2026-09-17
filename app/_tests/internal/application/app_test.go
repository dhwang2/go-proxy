package application

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go-proxy/internal/config"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
)

func TestOperationLockRejectsContenderPromptly(t *testing.T) {
	a := protocolTestApp(t)
	ctx := context.Background()
	contender := New(nil)
	contender.LockDir, contender.RequireRoot = a.LockDir, false
	_, err := a.Operation(ctx, func() (Result, error) {
		started := time.Now()
		_, err := contender.Operation(ctx, func() (Result, error) {
			t.Fatal("contending mutation entered its operation")
			return Result{}, nil
		})
		var detail *Error
		if !errors.As(err, &detail) || detail.Code != "busy" {
			t.Fatalf("contender error: %v", err)
		}
		if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
			t.Fatalf("busy operation waited %s", elapsed)
		}
		return Result{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStateLockRejectsReadersAndWritersPromptly(t *testing.T) {
	a := protocolTestApp(t)
	ctx := context.Background()
	err := a.State(ctx, func() error {
		for _, attempt := range []func() error{
			func() error { _, err := a.Snapshot(ctx); return err },
			func() error {
				return a.State(ctx, func() error { t.Fatal("contending state writer entered"); return nil })
			},
		} {
			started := time.Now()
			err := attempt()
			var detail *Error
			if !errors.As(err, &detail) || detail.Code != "busy" {
				t.Fatalf("state contention: %v", err)
			}
			if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
				t.Fatalf("busy state waited %s", elapsed)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestIsolatedPreparationKeepsSnapshotsReadable(t *testing.T) {
	a := protocolTestApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	preparing, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := a.Operation(ctx, func() (Result, error) {
			if _, err := a.Snapshot(ctx); err != nil {
				return Result{}, err
			}
			close(preparing)
			select {
			case <-release:
				return Result{}, nil
			case <-ctx.Done():
				return Result{}, ctx.Err()
			}
		})
		done <- err
	}()
	defer func() {
		close(release)
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("prepared operation: %v", err)
			}
		case <-ctx.Done():
			t.Error("operation did not release after preparation")
		}
	}()
	select {
	case <-preparing:
	case <-ctx.Done():
		t.Fatal("operation did not reach preparation")
	}
	for i := 0; i < 4; i++ {
		if _, err := a.Snapshot(ctx); err != nil {
			t.Fatalf("reader %d blocked by preparation: %v", i, err)
		}
	}
	select {
	case err := <-done:
		t.Fatalf("operation ended before preparation release: %v", err)
	default:
	}
}

func TestSnapshotReleasesStateLockBeforeReturning(t *testing.T) {
	a := protocolTestApp(t)
	ctx := context.Background()
	if _, err := a.Snapshot(ctx); err != nil {
		t.Fatal(err)
	}
	_, err := a.Operation(ctx, func() (Result, error) {
		return Result{}, a.State(ctx, func() error { return nil })
	})
	if err != nil {
		t.Fatalf("completed snapshot retained a read lock: %v", err)
	}
}

func TestCommitRejectsExternalChangesWithoutOverwritingThem(t *testing.T) {
	a := protocolTestApp(t)
	ctx := context.Background()
	external := []byte(`{"schema":3,"groups":{"default":["external-user"]}}`)
	_, err := a.Operation(ctx, func() (Result, error) {
		snapshot, err := a.Snapshot(ctx)
		if err != nil {
			return Result{}, err
		}
		snapshot.Store.UserMeta.Groups["default"] = []string{"prepared-user"}
		snapshot.Store.MarkDirty(store.FileUserMeta)
		if err := os.WriteFile(config.UserMetaFile, external, 0600); err != nil {
			return Result{}, err
		}
		err = a.Commit(ctx, snapshot)
		var detail *Error
		if !errors.As(err, &detail) || detail.Code != "conflict" || detail.Changed || snapshot.committed {
			t.Fatalf("external conflict: %v, committed=%v", err, snapshot.committed)
		}
		return Result{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(config.UserMetaFile)
	if err != nil || !bytes.Equal(data, external) {
		t.Fatalf("external state was overwritten: %s (%v)", data, err)
	}
	if _, err := os.Stat(filepath.Join(a.LockDir, "pending-services.json")); !os.IsNotExist(err) {
		t.Fatalf("conflict wrote activation state: %v", err)
	}
}

func TestValidationFailureNeverWritesCandidateConfiguration(t *testing.T) {
	a := protocolTestApp(t)
	oldBinary := config.SingBoxBin
	config.SingBoxBin = filepath.Join(t.TempDir(), "reject-config")
	t.Cleanup(func() { config.SingBoxBin = oldBinary })
	if err := os.WriteFile(config.SingBoxBin, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(config.SingBoxConfig)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Operation(context.Background(), func() (Result, error) {
		snapshot, err := a.Snapshot(context.Background())
		if err != nil {
			return Result{}, err
		}
		snapshot.Store.SingBox.Inbounds = []store.Inbound{{Type: "vless", Tag: "prepared", ListenPort: 24443}}
		snapshot.Store.UserMeta.Groups["default"] = []string{"prepared-user"}
		snapshot.Store.MarkDirty(store.FileSingBox)
		snapshot.Store.MarkDirty(store.FileUserMeta)
		err = a.Commit(context.Background(), snapshot)
		var detail *Error
		if !errors.As(err, &detail) || detail.Code != "validation_failed" || detail.Changed || snapshot.committed {
			t.Fatalf("validation result: %v", err)
		}
		return Result{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(config.SingBoxConfig)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("validator failure changed live configuration: %s (%v)", after, err)
	}
	if _, err := os.Stat(config.UserMetaFile); !os.IsNotExist(err) {
		t.Fatalf("validator failure wrote user state: %v", err)
	}
	if _, err := os.Stat(filepath.Join(a.LockDir, "pending-services.json")); !os.IsNotExist(err) {
		t.Fatalf("validator failure wrote activation state: %v", err)
	}
}

func TestActivationFailureReportsCommittedStateAndKeepsRetryIntent(t *testing.T) {
	a := protocolTestApp(t)
	toolsDir := t.TempDir()
	oldBinary := config.SingBoxBin
	config.SingBoxBin = filepath.Join(toolsDir, "sing-box")
	t.Cleanup(func() { config.SingBoxBin = oldBinary })
	for path, body := range map[string]string{
		config.SingBoxBin:                    "#!/bin/sh\nexit 0\n",
		filepath.Join(toolsDir, "systemctl"): "#!/bin/sh\nexit 1\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", toolsDir)
	ctx := context.Background()
	_, err := a.Operation(ctx, func() (Result, error) {
		snapshot, err := a.Snapshot(ctx)
		if err != nil {
			return Result{}, err
		}
		snapshot.Store.SingBox.Inbounds = []store.Inbound{{Type: "vless", Tag: "committed-node", ListenPort: 24443}}
		snapshot.Store.MarkDirty(store.FileSingBox)
		if err := a.Commit(ctx, snapshot); err != nil {
			return Result{}, err
		}
		err = a.Activate(ctx, snapshot, service.SingBox)
		var detail *Error
		if !errors.As(err, &detail) || !detail.Changed || detail.Stage != "activate" {
			t.Fatalf("activation failure understated the mutation: %v", err)
		}
		return Result{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Store.SingBox.Inbounds) != 1 || snapshot.Store.SingBox.Inbounds[0].Tag != "committed-node" {
		t.Fatal("activation failure pretended to roll back committed configuration")
	}
	pending, err := a.pendingServices()
	if err != nil || !pending[service.SingBox] {
		t.Fatalf("activation failure lost retry intent: %#v (%v)", pending, err)
	}
}

func TestFailedAndCancelledOperationsReleaseLocks(t *testing.T) {
	a := protocolTestApp(t)
	ctx := context.Background()
	failure := errors.New("preparation failed")
	if _, err := a.Operation(ctx, func() (Result, error) { return Result{}, failure }); !errors.Is(err, failure) {
		t.Fatalf("operation failure: %v", err)
	}
	if err := a.State(ctx, func() error { return failure }); !errors.Is(err, failure) {
		t.Fatalf("state failure: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := a.Operation(cancelled, func() (Result, error) { t.Fatal("cancelled operation entered"); return Result{}, nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled operation: %v", err)
	}
	if _, err := a.Operation(ctx, func() (Result, error) { return Result{}, a.State(ctx, func() error { return nil }) }); err != nil {
		t.Fatalf("failure leaked a lock: %v", err)
	}
}
