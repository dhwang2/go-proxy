package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"go-proxy/internal/config"
	"go-proxy/internal/store"
)

const defaultShadowTLSUnitDir = "/etc/systemd/system"

var shadowTLSUnitDir = defaultShadowTLSUnitDir

type ShadowTLSBinding struct {
	ServiceName  string
	ServicePath  string
	ListenPort   int
	BackendPort  int
	BackendProto string
	SNI          string
	Password     string
	Version      int
}

func ShadowTLSServiceName(backendProto string, backendPort int) string {
	backendProto = strings.ToLower(strings.TrimSpace(backendProto))
	var b strings.Builder
	for _, r := range backendProto {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	return fmt.Sprintf("shadow-tls-%s-%d", name, backendPort)
}

func ProvisionShadowTLSBinding(ctx context.Context, backendProto string, listenPort int, password, sni string, backendPort int) (string, error) {
	backendProto = strings.ToLower(strings.TrimSpace(backendProto))
	if backendProto != "ss" && backendProto != "snell" {
		return "", fmt.Errorf("invalid shadow-tls backend: %s", backendProto)
	}
	serviceName := ShadowTLSServiceName(backendProto, backendPort)
	if err := writeShadowTLSUnit(ctx, shadowTLSServicePath(serviceName), shadowTLSServiceLog(serviceName), backendProto, listenPort, password, sni, backendPort); err != nil {
		return "", err
	}
	return serviceName, nil
}

func FindShadowTLSBindingByBackend(s *store.Store, backendProto string, backendPort int) (*ShadowTLSBinding, error) {
	bindings, err := ListShadowTLSBindings(s)
	if err != nil {
		return nil, err
	}
	for i := range bindings {
		if bindings[i].BackendProto == backendProto && bindings[i].BackendPort == backendPort {
			return &bindings[i], nil
		}
	}
	return nil, nil
}

func RemoveShadowTLSBindingByBackend(ctx context.Context, backendProto string, backendPort int) error {
	backendProto = strings.ToLower(strings.TrimSpace(backendProto))
	if backendProto != "ss" && backendProto != "snell" {
		return nil
	}
	serviceName := ShadowTLSServiceName(backendProto, backendPort)
	unitPath := shadowTLSServicePath(serviceName)
	if _, err := os.Stat(unitPath); os.IsNotExist(err) {
		return nil
	}

	_ = Stop(ctx, Name(serviceName))
	_ = Disable(ctx, Name(serviceName))
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	logPath := shadowTLSServiceLog(serviceName)
	if err := os.Remove(logPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := DaemonReload(ctx); err != nil {
		if shadowTLSUnitDir != defaultShadowTLSUnitDir {
			return nil
		}
		return err
	}
	return nil
}

func ListShadowTLSBindings(s *store.Store) ([]ShadowTLSBinding, error) {
	_ = s
	paths, err := shadowTLSUnitPaths()
	if err != nil {
		return nil, err
	}

	bindings := make([]ShadowTLSBinding, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		serviceName := strings.TrimSuffix(filepath.Base(path), ".service")
		binding, ok := parseShadowTLSBinding(serviceName, path, string(data))
		if !ok {
			continue
		}
		if binding.BackendProto == "" || binding.BackendProto == "unknown" {
			binding.BackendProto = shadowTLSBackendProtoFromServiceName(serviceName)
		}
		if binding.BackendProto == "" || binding.BackendProto == "unknown" {
			continue
		}
		bindings = append(bindings, binding)
	}

	sort.Slice(bindings, func(i, j int) bool {
		if bindings[i].ListenPort != bindings[j].ListenPort {
			return bindings[i].ListenPort < bindings[j].ListenPort
		}
		return bindings[i].ServiceName < bindings[j].ServiceName
	})
	return bindings, nil
}

func ShadowTLSServiceNames() ([]string, error) {
	paths, err := shadowTLSUnitPaths()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(paths))
	for _, path := range paths {
		names = append(names, strings.TrimSuffix(filepath.Base(path), ".service"))
	}
	return names, nil
}

func writeShadowTLSUnit(ctx context.Context, unitPath, logPath, backendProto string, listenPort int, password, sni string, backendPort int) error {
	unit := fmt.Sprintf(`[Unit]
Description=Shadow-TLS v3 Service
After=network.target

[Service]
Type=simple
Environment=GPROXY_SHADOWTLS_BACKEND=%s
Environment=GPROXY_SHADOWTLS_BACKEND_PORT=%d
ExecStart=%s --v3 server --listen 0.0.0.0:%d --server 127.0.0.1:%d --tls %s --password %s
Restart=on-failure
RestartSec=10s
StandardOutput=append:%s
StandardError=append:%s

[Install]
WantedBy=multi-user.target
`, backendProto, backendPort, config.ShadowTLSBin, listenPort, backendPort, sni, password, logPath, logPath)
	return provisionUnit(ctx, unitPath, unit)
}

func shadowTLSUnitPaths() ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(shadowTLSUnitDir, "shadow-tls-*.service"))
	if err != nil {
		return nil, err
	}
	paths := append([]string(nil), matches...)
	sort.Strings(paths)
	return paths, nil
}

func shadowTLSServicePath(serviceName string) string {
	return filepath.Join(shadowTLSUnitDir, serviceName+".service")
}

func shadowTLSServiceLog(serviceName string) string {
	return filepath.Join(config.LogDir, serviceName+".service.log")
}

func parseShadowTLSBinding(serviceName, servicePath, unit string) (ShadowTLSBinding, bool) {
	binding := ShadowTLSBinding{
		ServiceName: serviceName,
		ServicePath: servicePath,
		Version:     2,
	}
	execStart := ""
	for _, line := range strings.Split(unit, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ExecStart=") {
			execStart = strings.TrimSpace(strings.TrimPrefix(line, "ExecStart="))
			continue
		}
		if strings.HasPrefix(line, "Environment=") {
			envLine := strings.TrimSpace(strings.TrimPrefix(line, "Environment="))
			switch {
			case strings.HasPrefix(envLine, "GPROXY_SHADOWTLS_BACKEND="):
				binding.BackendProto = strings.TrimSpace(strings.TrimPrefix(envLine, "GPROXY_SHADOWTLS_BACKEND="))
			case strings.HasPrefix(envLine, "GPROXY_SHADOWTLS_BACKEND_PORT="):
				if port := parsePortArg(strings.TrimSpace(strings.TrimPrefix(envLine, "GPROXY_SHADOWTLS_BACKEND_PORT="))); port > 0 {
					binding.BackendPort = port
				}
			}
		}
	}
	if execStart == "" {
		return ShadowTLSBinding{}, false
	}

	fields := strings.Fields(execStart)
	if len(fields) == 0 {
		return ShadowTLSBinding{}, false
	}

	if strings.Contains(execStart, " --v3 ") || strings.HasSuffix(execStart, " --v3") || strings.HasPrefix(execStart, "--v3 ") {
		binding.Version = 3
	}

	for i := 0; i < len(fields); i++ {
		if i+1 >= len(fields) {
			continue
		}
		switch fields[i] {
		case "--listen":
			binding.ListenPort = parsePortArg(fields[i+1])
		case "--server":
			if binding.BackendPort == 0 {
				binding.BackendPort = parsePortArg(fields[i+1])
			}
		case "--tls":
			binding.SNI = fields[i+1]
		case "--password":
			binding.Password = fields[i+1]
		}
	}

	if binding.ListenPort == 0 || binding.BackendPort == 0 {
		return ShadowTLSBinding{}, false
	}
	return binding, true
}

func parsePortArg(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if idx := strings.LastIndex(value, ":"); idx >= 0 {
		value = value[idx+1:]
	}
	port, _ := strconv.Atoi(value)
	return port
}

func shadowTLSBackendProtoFromServiceName(serviceName string) string {
	if !strings.HasPrefix(serviceName, "shadow-tls-") {
		return ""
	}
	rest := strings.TrimPrefix(serviceName, "shadow-tls-")
	if idx := strings.LastIndex(rest, "-"); idx > 0 {
		switch rest[:idx] {
		case "ss", "snell":
			return rest[:idx]
		}
	}
	return ""
}
