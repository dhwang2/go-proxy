package cert

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go-proxy/internal/config"
	"go-proxy/internal/core"
	"go-proxy/internal/network"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
)

var domainRe = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

const certIssueTimeout = 5 * time.Minute

// IsValidDomain checks if the string is a valid domain name (not an IP).
func IsValidDomain(domain string) bool {
	if domain == "" {
		return false
	}
	if net.ParseIP(domain) != nil {
		return false
	}
	return domainRe.MatchString(domain)
}

// ReadDomain reads the stored domain from .domain file.
func ReadDomain() string {
	data, err := os.ReadFile(config.DomainFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// WriteDomain writes the domain to .domain file.
func WriteDomain(domain string) error {
	return os.WriteFile(config.DomainFile, []byte(domain+"\n"), 0644)
}

// DefaultEmail returns a default Let's Encrypt email address.
func DefaultEmail(_ string) string {
	return "user@gmail.com"
}

// GenerateCaddyfile creates a Caddyfile for TLS certificate issuance.
func GenerateCaddyfile(domain, email string) error {
	if email == "" {
		email = DefaultEmail(domain)
	}
	content := fmt.Sprintf(`{
    email %s
    auto_https disable_redirects
}

%s:18443 {
    tls {
        protocols tls1.2 tls1.3
    }
    respond "ok" 200
}
`, email, domain)
	return os.WriteFile(config.CaddyFile, []byte(content), 0644)
}

// CertExists checks if TLS certificate files exist for the given domain
// under any ACME issuer directory in CaddyCertDir.
func CertExists(domain string) bool {
	certFile, keyFile := findCertPair(domain)
	return certFile != "" && keyFile != ""
}

// WaitForCert polls for certificate files until they appear or timeout.
func WaitForCert(ctx context.Context, domain string, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("证书申请超时 (%v)，请检查域名 DNS 是否指向服务器", timeout)
		case <-ticker.C:
			if CertExists(domain) {
				return nil
			}
		}
	}
}

// RestartCaddySub restarts the caddy-sub systemd service.
func RestartCaddySub(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "systemctl", "restart", "caddy-sub")
	return cmd.Run()
}

func WaitForCaddySub(ctx context.Context) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(10 * time.Second)
	for {
		if exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", "caddy-sub").Run() == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("证书入口启动失败，请检查 caddy 日志")
		case <-ticker.C:
		}
	}
}

func findCertPair(domain string) (string, string) {
	if domain == "" {
		return "", ""
	}

	var certFile string
	var keyFile string
	_ = filepath.WalkDir(config.CaddyCertDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if filepath.Base(path) != domain+".crt" || filepath.Base(filepath.Dir(path)) != domain {
			return nil
		}
		keyCandidate := strings.TrimSuffix(path, ".crt") + ".key"
		if _, err := os.Stat(keyCandidate); err != nil {
			return nil
		}
		certFile = path
		keyFile = keyCandidate
		return fs.SkipAll
	})
	return certFile, keyFile
}

func domainPointsToThisServer(ctx context.Context, domain string) bool {
	if domain == "" {
		return true
	}

	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	addrs, err := net.DefaultResolver.LookupIPAddr(lookupCtx, domain)
	if err != nil || len(addrs) == 0 {
		return false
	}

	localV4 := detectPublicCertIP("https://api.ipify.org", "https://ipv4.icanhazip.com")
	localV6 := detectPublicCertIP("https://api64.ipify.org", "https://icanhazip.com")
	if localV4 == "" && localV6 == "" {
		return true
	}

	for _, addr := range addrs {
		ip := addr.IP
		switch ip.String() {
		case localV4, localV6:
			return true
		}
	}
	return false
}

func detectPublicCertIP(endpoints ...string) string {
	client := &http.Client{Timeout: 2 * time.Second}
	for _, endpoint := range endpoints {
		resp, err := client.Get(endpoint)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
		resp.Body.Close()
		if err != nil {
			continue
		}
		ip := net.ParseIP(strings.TrimSpace(string(body)))
		if ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() {
			return ip.String()
		}
	}
	return ""
}

func refreshManagedFirewall() error {
	if !network.HasManagedConvergence() {
		return nil
	}
	s, err := store.Load()
	if err != nil {
		return err
	}
	return network.ApplyConvergence(s)
}

// EnsureCertificate orchestrates the full certificate issuance flow:
// check existing -> ensure caddy binary -> ensure caddy-sub unit -> write domain ->
// generate Caddyfile -> restart caddy -> wait for cert.
// The optional progress callback is called with status strings during the flow.
func EnsureCertificate(ctx context.Context, domain, email string, progress func(string)) error {
	if CertExists(domain) {
		return nil
	}

	if progress != nil {
		progress("正在检查域名解析...")
	}
	if !domainPointsToThisServer(ctx, domain) {
		return fmt.Errorf("证书申请前检查失败：域名当前未解析到本机，请先更新 A/AAAA 记录，再重试")
	}

	// Step 1: Ensure caddy binary exists.
	if progress != nil {
		progress("正在检查 caddy...")
	}
	if _, err := os.Stat(config.CaddyBin); os.IsNotExist(err) {
		if progress != nil {
			progress("正在安装 caddy...")
		}
		if err := downloadCaddy(ctx); err != nil {
			return fmt.Errorf("caddy 安装失败: %w", err)
		}
	}

	// Step 2: Ensure caddy-sub systemd unit exists.
	if _, err := os.Stat(config.CaddySubService); os.IsNotExist(err) {
		if progress != nil {
			progress("正在配置 caddy 服务...")
		}
		if err := service.ProvisionCaddySub(ctx); err != nil {
			return fmt.Errorf("caddy 服务配置失败: %w", err)
		}
	}

	// Step 3: Write domain file.
	if progress != nil {
		progress("正在配置域名...")
	}
	if err := WriteDomain(domain); err != nil {
		return fmt.Errorf("写入域名文件失败: %w", err)
	}

	// Step 4: Generate Caddyfile.
	if progress != nil {
		progress("正在生成证书配置...")
	}
	if err := GenerateCaddyfile(domain, email); err != nil {
		return fmt.Errorf("生成 Caddyfile 失败: %w", err)
	}
	if err := refreshManagedFirewall(); err != nil {
		return fmt.Errorf("更新防火墙收敛失败: %w", err)
	}

	// Step 5: Restart caddy-sub.
	if progress != nil {
		progress("正在启动证书服务...")
	}
	if err := RestartCaddySub(ctx); err != nil {
		return fmt.Errorf("重启 caddy-sub 失败: %w", err)
	}
	if err := WaitForCaddySub(ctx); err != nil {
		return err
	}

	// Step 6: Wait for cert.
	if progress != nil {
		progress("等待证书签发...")
	}
	return WaitForCert(ctx, domain, certIssueTimeout)
}

// downloadCaddy downloads the caddy binary using the GitHub release mechanism.
func downloadCaddy(ctx context.Context) error {
	check, err := core.CheckUpdate(ctx, core.CompCaddy, config.CaddyBin)
	if err != nil {
		return fmt.Errorf("check caddy release: %w", err)
	}
	if check.DownloadURL == "" {
		return fmt.Errorf("no download URL found for caddy")
	}
	return core.DownloadBinary(ctx, check.DownloadURL, config.CaddyBin)
}
