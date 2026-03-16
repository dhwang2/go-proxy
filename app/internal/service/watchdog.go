package service

import (
	"context"
	"log"
	"time"
)

// WatchdogConfig configures the health check watchdog.
type WatchdogConfig struct {
	Interval    time.Duration
	MaxFailures int
	Services    []Name
}

// DefaultWatchdogConfig returns the default watchdog configuration.
func DefaultWatchdogConfig() WatchdogConfig {
	return WatchdogConfig{
		Interval:    30 * time.Second,
		MaxFailures: 3,
		Services:    []Name{SingBox},
	}
}

// RunWatchdog starts the health check loop. It blocks until ctx is cancelled.
func RunWatchdog(ctx context.Context, cfg WatchdogConfig) error {
	failures := make(map[Name]int)
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			for _, svc := range cfg.Services {
				st, err := GetStatus(ctx, svc)
				if err != nil {
					log.Printf("watchdog: failed to check %s: %v", svc, err)
					continue
				}
				if st.Running {
					failures[svc] = 0
					continue
				}
				failures[svc]++
				log.Printf("watchdog: %s not running (failure %d/%d)",
					svc, failures[svc], cfg.MaxFailures)
				if failures[svc] >= cfg.MaxFailures {
					log.Printf("watchdog: restarting %s", svc)
					if err := Restart(ctx, svc); err != nil {
						log.Printf("watchdog: failed to restart %s: %v", svc, err)
					} else {
						failures[svc] = 0
					}
				}
			}
		}
	}
}
