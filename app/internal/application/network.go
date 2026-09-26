package application

import (
	"context"
	"os"
	"reflect"
	"slices"
	"strings"

	"go-proxy/internal/config"
	"go-proxy/internal/network"
	"go-proxy/internal/store"
	"go-proxy/pkg/fileutil"
)

// NetworkBBR reads, enables or disables BBR. Every answer says what keeps it
// at boot: gproxy's own file (managed) and any other sysctl file that sets it
// (boot_sources), so a disable that another file will undo says so.
func (a *App) NetworkBBR(ctx context.Context, action string) (Result, error) {
	report := func(change string) (map[string]any, error) {
		enabled, current, err := network.BBRStatus()
		data := map[string]any{"enabled": enabled, "current": current, "managed": network.BBRManaged(), "boot_sources": network.BBRBootSources()}
		if change != "" {
			data["change"] = change
		}
		return data, err
	}
	if action == "status" {
		data, err := report("")
		return Result{Data: data}, err
	}
	if action != "enable" && action != "disable" {
		return Result{}, Invalid("unknown bbr action")
	}
	return a.Operation(ctx, func() (Result, error) {
		before, _, err := network.BBRStatus()
		if err != nil {
			return Result{}, err
		}
		if action == "disable" {
			// Nothing of gproxy's and nothing running: no work, no progress line.
			if !before && !network.BBRManaged() {
				data, err := report("already disabled")
				return Result{Data: data}, err
			}
			a.Progress("disabling bbr")
			if err := network.DisableBBR(ctx); err != nil {
				return Result{}, &Error{Code: "activation_failed", Message: err.Error(), Stage: "sysctl", Changed: true}
			}
			data, err := report("disabled")
			if err != nil {
				return Result{}, &Error{Code: "activation_failed", Message: err.Error(), Stage: "verify", Changed: true}
			}
			if data["enabled"] == true {
				return Result{}, &Error{Code: "activation_failed", Message: "bbr is still active", Stage: "verify", Changed: true}
			}
			return Result{Changed: true, Data: data}, nil
		}
		persisted, readErr := os.ReadFile(network.BBRSysctlPath)
		if readErr != nil && !os.IsNotExist(readErr) {
			return Result{}, readErr
		}
		changed := !before || string(persisted) != "net.core.default_qdisc = fq\nnet.ipv4.tcp_congestion_control = bbr\n"
		a.Progress("enabling bbr")
		if err := network.EnableBBR(ctx); err != nil {
			return Result{}, &Error{Code: "activation_failed", Message: err.Error(), Stage: "sysctl", Changed: true}
		}
		change := "enabled"
		if !changed {
			change = "already enabled"
		}
		data, err := report(change)
		if err != nil {
			return Result{}, &Error{Code: "activation_failed", Message: err.Error(), Stage: "verify", Changed: true}
		}
		if data["enabled"] != true {
			return Result{}, &Error{Code: "activation_failed", Message: "bbr is not active", Stage: "verify", Changed: true}
		}
		return Result{Changed: changed, Data: data}, nil
	})
}

func (a *App) NetworkFirewallStatus(ctx context.Context) (Result, error) {
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		return Result{}, err
	}
	observationCtx, cancel := ObservationContext(ctx)
	defer cancel()
	info, err := network.FirewallStatus(observationCtx, snapshot.Store, snapshot.Bindings)
	return Result{Data: info}, err
}

func (a *App) NetworkFirewallApply(ctx context.Context) (Result, error) {
	return a.Operation(ctx, func() (Result, error) {
		a.Progress("applying managed firewall")
		snapshot, err := a.Snapshot(ctx)
		if err != nil {
			return Result{}, err
		}
		if err := network.ApplyFirewallConvergence(ctx, snapshot.Store); err != nil {
			return Result{}, &Error{Code: "activation_failed", Message: err.Error(), Stage: "firewall", Changed: true}
		}
		info, err := network.FirewallStatus(ctx, snapshot.Store, snapshot.Bindings)
		if err != nil {
			return Result{}, &Error{Code: "activation_failed", Message: err.Error(), Stage: "verify", Changed: true}
		}
		return Result{Changed: true, Data: info}, nil
	})
}

func (a *App) NetworkFirewallClear(ctx context.Context) (Result, error) {
	return a.Operation(ctx, func() (Result, error) {
		managed, err := network.FirewallManaged(ctx)
		if err != nil {
			return Result{}, err
		}
		if !managed {
			// removed says which of the two happened: there was no managed table
			// to release, which is the same envelope otherwise.
			return Result{Data: map[string]any{"managed": false, "removed": false}}, nil
		}
		a.Progress("removing managed firewall")
		if err := network.RemoveFirewallRules(ctx); err != nil {
			return Result{}, &Error{Code: "activation_failed", Message: err.Error(), Stage: "firewall", Changed: true}
		}
		return Result{Changed: true, Data: map[string]any{"managed": false, "removed": true}}, nil
	})
}

// FirewallPortChange is what one custom-port request did to one transport:
// added, already added, removed or not found.
type FirewallPortChange struct {
	Port      int    `json:"port"`
	Transport string `json:"transport"`
	Result    string `json:"result"`
}

// FirewallTransports lists the accepted custom-port transports, shared by the
// validator below and shell completion.
func FirewallTransports() []string { return []string{"tcp", "udp", "both"} }

func (a *App) NetworkFirewallPort(ctx context.Context, port int, transport string, remove bool) (Result, error) {
	if port < 1 || port > 65535 {
		return Result{}, Invalid("port must be between 1 and 65535")
	}
	if !slices.Contains(FirewallTransports(), transport) {
		return Result{}, Invalid("transport must be tcp, udp or both")
	}
	return a.Operation(ctx, func() (Result, error) {
		snapshot, err := a.Snapshot(ctx)
		if err != nil {
			return Result{}, err
		}
		managed, err := network.FirewallManaged(ctx)
		if err != nil {
			return Result{}, err
		}
		s := snapshot.Store
		if s.Firewall == nil {
			s.Firewall = &store.FirewallConfig{}
		}
		before := append([]store.FirewallPort(nil), s.Firewall.Ports...)
		// What happened to each transport the argument named, so the reply
		// can say it per port: "both" is two entries.
		changes := []FirewallPortChange{}
		for _, proto := range []string{"tcp", "udp"} {
			if transport != proto && transport != "both" {
				continue
			}
			had := slices.Contains(before, store.FirewallPort{Proto: proto, Port: port})
			var result string
			switch {
			case remove && had:
				result = "removed"
			case remove:
				result = "not found"
			case had:
				result = "already added"
			default:
				result = "added"
			}
			changes = append(changes, FirewallPortChange{Port: port, Transport: proto, Result: result})
		}
		if remove {
			kept := s.Firewall.Ports[:0]
			for _, entry := range s.Firewall.Ports {
				if entry.Port != port || (transport != "both" && entry.Proto != transport) {
					kept = append(kept, entry)
				}
			}
			s.Firewall.Ports = kept
		} else {
			for _, proto := range []string{"tcp", "udp"} {
				if transport == proto || transport == "both" {
					s.Firewall.Ports = append(s.Firewall.Ports, store.FirewallPort{Proto: proto, Port: port})
				}
			}
		}
		s.Firewall.Normalize()
		changed := !reflect.DeepEqual(before, s.Firewall.Ports)
		if changed {
			s.MarkDirty(store.FileFirewall)
			if err := a.Commit(ctx, snapshot); err != nil {
				return Result{}, err
			}
		}
		if managed {
			a.Progress("reconciling managed firewall")
			if err := network.ApplyFirewallConvergence(ctx, s); err != nil {
				return Result{}, &Error{Code: "activation_failed", Message: err.Error(), Stage: "firewall", Changed: changed, Data: map[string]any{"pending": "firewall convergence"}}
			}
		}
		ports := s.Firewall.Ports
		if ports == nil {
			ports = []store.FirewallPort{}
		}
		return Result{Changed: changed, Data: map[string]any{"ports": ports, "managed": managed, "changes": changes}}, nil
	})
}

func (a *App) NetworkFail2Ban(ctx context.Context, action string) (Result, error) {
	if action == "status" {
		observationCtx, cancel := ObservationContext(ctx)
		defer cancel()
		info, err := network.Fail2BanStatus(observationCtx)
		if err == nil {
			recordBanned(&info)
		}
		return Result{Data: info}, err
	}
	return a.Operation(ctx, func() (Result, error) {
		before, err := network.Fail2BanStatus(ctx)
		if err != nil {
			return Result{}, err
		}
		// Already off: said in the result, with no progress line claiming
		// work that did not happen.
		if action == "disable" && !before.Running && !before.StartsAtBoot && !before.Managed {
			recordBanned(&before)
			before.Change = "already stopped"
			return Result{Data: before}, nil
		}
		changed, change := true, "added"
		switch action {
		case "enable":
			switch {
			case !before.Running:
				change = "started"
			case before.Managed:
				// Written again so an edited file is put back, but the jail
				// was already gproxy's.
				changed, change = false, "already managed"
			}
			a.Progress("enabling the fail2ban ssh jail")
			err = network.Fail2BanEnable(ctx)
		case "disable":
			change = "stopped"
			a.Progress("stopping fail2ban")
			err = network.Fail2BanStop(ctx)
		default:
			return Result{}, Invalid("unknown fail2ban action")
		}
		if err != nil {
			return Result{}, &Error{Code: "activation_failed", Message: err.Error(), Stage: "fail2ban", Changed: true}
		}
		info, err := network.Fail2BanStatus(ctx)
		if err != nil {
			return Result{}, &Error{Code: "activation_failed", Message: err.Error(), Stage: "verify", Changed: true}
		}
		if action == "enable" && (!info.Running || !info.SSHJailEnabled) {
			return Result{}, &Error{Code: "activation_failed", Message: "fail2ban sshd jail is not active", Stage: "verify", Changed: true}
		}
		if action == "disable" {
			if info.Running {
				return Result{}, &Error{Code: "activation_failed", Message: "fail2ban is still running", Stage: "verify", Changed: true}
			}
			info.BansLifted = before.CurrentlyBanned
		}
		recordBanned(&info)
		info.Change = change
		return Result{Changed: changed, Data: info}, nil
	})
}

// recordBanned writes the banned addresses to their file, one per line, and
// names the file in the result. It is a report rather than state: rewritten
// whole on every call, empty when nothing is banned. A write that fails leaves
// the path out, and the list still travels in the result.
func recordBanned(info *network.Fail2BanInfo) {
	if !info.Installed {
		return
	}
	content := strings.Join(info.BannedIPs, "\n")
	if content != "" {
		content += "\n"
	}
	if err := fileutil.AtomicWriteMode(config.Fail2BanBannedFile, []byte(content), 0o644); err == nil {
		info.BannedFile = config.Fail2BanBannedFile
	}
}
