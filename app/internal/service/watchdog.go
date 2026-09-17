package service

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type WatchdogConfig struct {
	Interval    time.Duration
	MaxFailures int
	Services    []Name
	Recover     func(context.Context, Name) error
	Report      func(string)
}

func DefaultWatchdogConfig() WatchdogConfig {
	return WatchdogConfig{Interval: 30 * time.Second, MaxFailures: 3, Services: []Name{SingBox, Snell, ShadowTLS, CaddySub}}
}
func RunWatchdog(ctx context.Context, cfg WatchdogConfig) error {
	failures := make(map[Name]int)
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			var names []Name
			for _, name := range cfg.Services {
				if name != ShadowTLS {
					names = append(names, name)
					continue
				}
				bindings, err := ShadowTLSServiceNames()
				if err != nil {
					if cfg.Report != nil {
						cfg.Report("watchdog: binding observation failed")
					}
					continue
				}
				for _, binding := range bindings {
					names = append(names, Name(binding))
				}
			}
			if len(names) == 0 {
				continue
			}
			observe, cancel := context.WithTimeout(ctx, 2*time.Second)
			states, err := Snapshot(observe, names...)
			cancel()
			if err != nil {
				if cfg.Report != nil {
					cfg.Report("watchdog: service observation failed")
				}
				continue
			}
			for _, st := range states {
				stopped := IsStopped(st.Name) || strings.HasPrefix(string(st.Name), "shadow-tls-") && IsStopped(ShadowTLS)
				if !st.Installed || !st.Enabled || st.Running || stopped {
					failures[st.Name] = 0
					continue
				}
				failures[st.Name]++
				if failures[st.Name] < cfg.MaxFailures {
					continue
				}
				if cfg.Report != nil {
					cfg.Report(fmt.Sprintf("watchdog: recovering %s", st.Name))
				}
				recoverCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				if cfg.Recover == nil {
					err = fmt.Errorf("watchdog recovery is not configured")
				} else {
					err = cfg.Recover(recoverCtx, st.Name)
				}
				cancel()
				if err == nil {
					failures[st.Name] = 0
				} else if cfg.Report != nil {
					cfg.Report(fmt.Sprintf("watchdog: %s recovery failed", st.Name))
				}
			}
		}
	}
}
