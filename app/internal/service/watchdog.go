package service

import (
	"context"
	"strings"

	"github.com/dhwang2/go-proxy/pkg/sysutil"
)

// WatchdogEnabled checks if the proxy-watchdog service is active.
func WatchdogEnabled(ctx context.Context) (bool, error) {
	state, err := sysutil.ServiceState(ctx, "proxy-watchdog.service")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(state) == "active", nil
}
