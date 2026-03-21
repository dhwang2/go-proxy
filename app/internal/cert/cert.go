package cert

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go-proxy/internal/config"
	"go-proxy/internal/core"
	"go-proxy/internal/service"
)

var domainRe = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

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

// DefaultEmail returns the default Let's Encrypt email for a domain,
// stripping any "www." prefix so the address is valid.
func DefaultEmail(domain string) string {
	host := strings.TrimPrefix(domain, "www.")
	return "admin@" + host
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
	issuers, err := os.ReadDir(config.CaddyCertDir)
	if err != nil {
		return false
	}
	for _, issuer := range issuers {
		if !issuer.IsDir() {
			continue
		}
		certDir := filepath.Join(config.CaddyCertDir, issuer.Name(), domain)
		certFile := filepath.Join(certDir, domain+".crt")
		keyFile := filepath.Join(certDir, domain+".key")
		_, errCert := os.Stat(certFile)
		_, errKey := os.Stat(keyFile)
		if errCert == nil && errKey == nil {
			return true
		}
	}
	return false
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

// EnsureCertificate orchestrates the full certificate issuance flow:
// check existing -> ensure caddy binary -> ensure caddy-sub unit -> write domain ->
// generate Caddyfile -> restart caddy -> wait for cert.
// The optional progress callback is called with status strings during the flow.
func EnsureCertificate(ctx context.Context, domain, email string, progress func(string)) error {
	if CertExists(domain) {
		return nil
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

	// Step 5: Restart caddy-sub.
	if progress != nil {
		progress("正在启动证书服务...")
	}
	if err := RestartCaddySub(ctx); err != nil {
		return fmt.Errorf("重启 caddy-sub 失败: %w", err)
	}

	// Step 6: Wait for cert.
	if progress != nil {
		progress("等待证书签发...")
	}
	return WaitForCert(ctx, domain, 120*time.Second)
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
