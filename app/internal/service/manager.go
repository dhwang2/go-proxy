package service

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dhwang2/go-proxy/internal/store"
	"github.com/dhwang2/go-proxy/pkg/sysutil"
)

type Options struct {
	WorkDir    string
	ConfigPath string
}

type Manager struct {
	options Options
}

type ServiceStatus struct {
	Name    string
	Unit    string
	State   string
	Version string
}

type OperationResult struct {
	Name    string
	Status  string
	Message string
}

type ServiceSpec struct {
	Name string
	Unit string
	Bin  string
}

func NewManager(opts Options) *Manager {
	if opts.WorkDir == "" {
		opts.WorkDir = "/etc/go-proxy"
	}
	return &Manager{options: opts}
}

func (m *Manager) AllStatuses(ctx context.Context) ([]ServiceStatus, error) {
	specs := m.serviceSpecs()
	rows := make([]ServiceStatus, 0, len(specs))
	for _, spec := range specs {
		state, err := sysutil.ServiceState(ctx, spec.Unit)
		if err != nil {
			state = "unknown"
		}
		rows = append(rows, ServiceStatus{
			Name:    spec.Name,
			Unit:    spec.Unit,
			State:   state,
			Version: m.versionFor(spec),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows, nil
}

func (m *Manager) OperateAll(ctx context.Context, action string) []OperationResult {
	specs := m.serviceSpecs()
	results := make([]OperationResult, 0, len(specs))
	for _, spec := range specs {
		// Skip services whose binary is not installed.
		if spec.Bin != "" && !fileExists(spec.Bin) {
			results = append(results, OperationResult{Name: spec.Name, Status: "skipped", Message: "not installed"})
			continue
		}
		// Skip services without a systemd unit file.
		unitPath := filepath.Join("/etc/systemd/system", spec.Unit)
		if !fileExists(unitPath) {
			results = append(results, OperationResult{Name: spec.Name, Status: "skipped", Message: "no service file"})
			continue
		}
		if spec.Unit == "sing-box.service" && m.options.ConfigPath != "" {
			if _, err := os.Stat(m.options.ConfigPath); errors.Is(err, os.ErrNotExist) {
				results = append(results, OperationResult{Name: spec.Name, Status: "skipped", Message: "config not found"})
				continue
			}
		}
		err := sysutil.ServiceAction(ctx, action, spec.Unit)
		if err != nil {
			results = append(results, OperationResult{Name: spec.Name, Status: "failed", Message: err.Error()})
			continue
		}
		results = append(results, OperationResult{Name: spec.Name, Status: "ok"})
	}
	return results
}

// ApplyStore persists dirty state and performs minimal required restarts.
// Current policy:
// - config changed: restart sing-box
// - snell changed:  restart snell-v5
func (m *Manager) ApplyStore(ctx context.Context, st *store.Store) []OperationResult {
	if st == nil {
		return []OperationResult{{Name: "store", Status: "failed", Message: "nil store"}}
	}
	configChanged := st.ConfigDirty()
	snellChanged := st.SnellDirty()
	metaChanged := st.UserMetaDirty()

	if !configChanged && !snellChanged && !metaChanged {
		return []OperationResult{{Name: "store", Status: "ok", Message: "no changes"}}
	}
	if err := st.Save(); err != nil {
		return []OperationResult{{Name: "store", Status: "failed", Message: err.Error()}}
	}

	results := make([]OperationResult, 0, 3)
	results = append(results, OperationResult{Name: "store", Status: "ok", Message: "persisted"})
	if configChanged {
		if m.options.ConfigPath != "" {
			checkBin := filepath.Join(m.options.WorkDir, "bin", "sing-box")
			if fileExists(checkBin) {
				out, err := exec.CommandContext(ctx, checkBin, "check", "-c", m.options.ConfigPath).CombinedOutput()
				if err != nil {
					results = append(results, OperationResult{Name: "sing-box", Status: "failed", Message: "config check: " + strings.TrimSpace(string(out))})
					return results
				}
			}
		}
		if err := sysutil.ServiceAction(ctx, "restart", "sing-box.service"); err != nil {
			results = append(results, OperationResult{Name: "sing-box", Status: "failed", Message: err.Error()})
		} else {
			results = append(results, OperationResult{Name: "sing-box", Status: "ok", Message: "restarted"})
		}
	}
	if snellChanged {
		if err := sysutil.ServiceAction(ctx, "restart", "snell-v5.service"); err != nil {
			results = append(results, OperationResult{Name: "snell-v5", Status: "failed", Message: err.Error()})
		} else {
			results = append(results, OperationResult{Name: "snell-v5", Status: "ok", Message: "restarted"})
		}
	}
	return results
}

func (m *Manager) PrintLogs(ctx context.Context, lines int, follow bool) error {
	if follow {
		if lines <= 0 {
			lines = 50
		}
		statuses, err := m.AllStatuses(ctx)
		if err != nil {
			return err
		}
		for _, s := range statuses {
			fmt.Printf("== %s ==\n", s.Name)
			journalArgs := []string{"-u", s.Unit, "--no-pager", "-n", fmt.Sprintf("%d", lines), "-f"}
			cmd := exec.CommandContext(ctx, "journalctl", journalArgs...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				fmt.Printf("(no logs: %v)\n\n", err)
				continue
			}
			fmt.Println(string(out))
		}
		return nil
	}
	text, err := m.CollectLogs(ctx, lines)
	if err != nil {
		return err
	}
	if strings.TrimSpace(text) != "" {
		fmt.Println(text)
	}
	return nil
}

func (m *Manager) CollectLogs(ctx context.Context, lines int) (string, error) {
	return m.CollectLogsFor(ctx, lines, "")
}

func (m *Manager) CollectLogsFor(ctx context.Context, lines int, serviceName string) (string, error) {
	if lines <= 0 {
		lines = 50
	}
	statuses, err := m.AllStatuses(ctx)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	for _, s := range statuses {
		if serviceName != "" && strings.TrimSpace(s.Name) != strings.TrimSpace(serviceName) {
			continue
		}
		out.WriteString("== " + s.Name + " ==\n")
		path := m.logPath(s.Unit)
		if path != "" {
			if text, err := tailFile(path, lines); err == nil && strings.TrimSpace(text) != "" {
				out.WriteString(text)
				out.WriteString("\n\n")
				continue
			}
		}
		journalArgs := []string{"-u", s.Unit, "--no-pager", "-n", fmt.Sprintf("%d", lines)}
		cmd := exec.CommandContext(ctx, "journalctl", journalArgs...)
		text, err := cmd.CombinedOutput()
		if err != nil {
			out.WriteString(fmt.Sprintf("(no logs: %v)\n\n", err))
			continue
		}
		out.WriteString(string(text))
		if !strings.HasSuffix(out.String(), "\n") {
			out.WriteString("\n")
		}
		out.WriteString("\n")
	}
	return strings.TrimRight(out.String(), "\n"), nil
}

func (m *Manager) serviceSpecs() []ServiceSpec {
	base := []ServiceSpec{
		{Name: "sing-box", Unit: "sing-box.service", Bin: filepath.Join(m.options.WorkDir, "bin", "sing-box")},
		{Name: "snell-v5", Unit: "snell-v5.service", Bin: filepath.Join(m.options.WorkDir, "bin", "snell-server")},
		{Name: "caddy-sub", Unit: "caddy-sub.service", Bin: filepath.Join(m.options.WorkDir, "bin", "caddy")},
		{Name: "proxy-watchdog", Unit: "proxy-watchdog.service"},
	}
	shadowSpecs := m.shadowTLSSpecs()
	base = append(base, shadowSpecs...)
	return base
}

func (m *Manager) shadowTLSSpecs() []ServiceSpec {
	units, _ := filepath.Glob("/etc/systemd/system/shadow-tls*.service")
	if len(units) == 0 {
		return []ServiceSpec{{Name: "shadow-tls", Unit: "shadow-tls.service", Bin: filepath.Join(m.options.WorkDir, "bin", "shadow-tls")}}
	}
	rows := make([]ServiceSpec, 0, len(units))
	for _, unitFile := range units {
		name := strings.TrimSuffix(filepath.Base(unitFile), ".service")
		rows = append(rows, ServiceSpec{
			Name: name,
			Unit: name + ".service",
			Bin:  filepath.Join(m.options.WorkDir, "bin", "shadow-tls"),
		})
	}
	return rows
}

func (m *Manager) versionFor(spec ServiceSpec) string {
	if spec.Bin == "" {
		return ""
	}
	if _, err := os.Stat(spec.Bin); err != nil {
		return ""
	}
	var out []byte
	var err error
	switch spec.Name {
	case "sing-box":
		out, err = exec.Command(spec.Bin, "version").CombinedOutput()
		if err != nil {
			return ""
		}
		fields := strings.Fields(string(out))
		if len(fields) >= 3 {
			return fields[2]
		}
		return strings.TrimSpace(string(out))
	case "snell-v5":
		out, err = exec.Command(spec.Bin, "-v").CombinedOutput()
		if err != nil {
			return ""
		}
		re := regexp.MustCompile(`v([0-9.]+)`)
		match := re.FindStringSubmatch(string(out))
		if len(match) > 1 {
			return match[1]
		}
		return ""
	case "caddy-sub":
		out, err = exec.Command(spec.Bin, "version").CombinedOutput()
		if err != nil {
			return ""
		}
		fields := strings.Fields(string(out))
		if len(fields) >= 1 {
			return strings.TrimPrefix(fields[0], "v")
		}
		return ""
	default:
		out, err = exec.Command(spec.Bin, "--version").CombinedOutput()
		if err != nil {
			return ""
		}
		fields := strings.Fields(string(out))
		if len(fields) == 0 {
			return ""
		}
		return strings.TrimPrefix(fields[len(fields)-1], "v")
	}
}

func (m *Manager) logPath(unit string) string {
	logDir := filepath.Join(m.options.WorkDir, "logs")
	switch unit {
	case "sing-box.service":
		if p := filepath.Join(logDir, "sing-box.log"); fileExists(p) {
			return p
		}
		return filepath.Join(logDir, "sing-box.service.log")
	case "snell-v5.service":
		return filepath.Join(logDir, "snell-v5.service.log")
	case "caddy-sub.service":
		return filepath.Join(logDir, "caddy-sub.service.log")
	case "proxy-watchdog.service":
		return filepath.Join(logDir, "proxy-watchdog.log")
	default:
		base := strings.TrimSuffix(strings.TrimSuffix(unit, ".service"), ".log")
		if p := filepath.Join(logDir, base+".log"); fileExists(p) {
			return p
		}
		return filepath.Join(logDir, "shadow-tls.service.log")
	}
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func tailFile(path string, lines int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	buf := make([]string, 0, lines)
	for s.Scan() {
		buf = append(buf, s.Text())
		if len(buf) > lines {
			buf = buf[1:]
		}
	}
	if err := s.Err(); err != nil {
		return "", err
	}
	var out bytes.Buffer
	for i, line := range buf {
		out.WriteString(line)
		if i < len(buf)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String(), nil
}
