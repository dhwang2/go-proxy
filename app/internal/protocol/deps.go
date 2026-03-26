package protocol

import (
	"context"
	"fmt"
	"os"

	"go-proxy/internal/config"
	"go-proxy/internal/core"
	"go-proxy/internal/service"
)

// DepStep describes one dependency provisioning step.
type DepStep struct {
	Description string
	Err         error
}

// ProvisionDeps ensures all runtime dependencies for a protocol are in place:
// binary downloaded, systemd service created, service enabled and started.
// It returns a slice of steps taken (for progress display).
func ProvisionDeps(ctx context.Context, protoType Type, params InstallParams) []DepStep {
	spec := specs[protoType]
	var steps []DepStep

	switch {
	case spec.SingBoxType != "" && spec.ExternalBin == "":
		// Managed by sing-box: ensure sing-box binary + service.
		steps = append(steps, ensureSingBox(ctx)...)

	case protoType == Snell:
		// Snell needs its own binary + service.
		steps = append(steps, ensureSnell(ctx)...)

	case protoType == ShadowTLS:
		// ShadowTLS provisioning is handled per backend binding during install.
		steps = append(steps, ensureShadowTLS(ctx, params)...)
	}

	return steps
}

func ensureSingBox(ctx context.Context) []DepStep {
	var steps []DepStep

	// Check if binary exists.
	if _, err := os.Stat(config.SingBoxBin); os.IsNotExist(err) {
		steps = append(steps, downloadComponent(ctx, core.CompSingBox, config.SingBoxBin))
	}

	// Ensure systemd service exists.
	if _, err := os.Stat(config.SingBoxService); os.IsNotExist(err) {
		step := DepStep{Description: "创建 sing-box 服务"}
		if err := service.ProvisionSingBox(ctx); err != nil {
			step.Err = err
			steps = append(steps, step)
			return steps
		}
		steps = append(steps, step)
	}

	// Enable and start.
	steps = append(steps, enableAndStart(ctx, service.SingBox)...)
	return steps
}

func ensureSnell(ctx context.Context) []DepStep {
	var steps []DepStep

	// Check if binary exists. Snell is private so we just check.
	if _, err := os.Stat(config.SnellBin); os.IsNotExist(err) {
		steps = append(steps, DepStep{
			Description: "下载 snell-server 二进制",
			Err:         fmt.Errorf("snell-server binary not found at %s; please install manually", config.SnellBin),
		})
		return steps
	}

	// Ensure systemd service.
	if _, err := os.Stat(config.SnellService); os.IsNotExist(err) {
		step := DepStep{Description: "创建 snell 服务"}
		if err := service.ProvisionSnell(ctx); err != nil {
			step.Err = err
			steps = append(steps, step)
			return steps
		}
		steps = append(steps, step)
	}

	steps = append(steps, enableAndStart(ctx, service.Snell)...)
	return steps
}

func ensureShadowTLS(ctx context.Context, params InstallParams) []DepStep {
	var steps []DepStep

	// Download shadow-tls binary if missing.
	if _, err := os.Stat(config.ShadowTLSBin); os.IsNotExist(err) {
		steps = append(steps, downloadComponent(ctx, core.CompShadowTLS, config.ShadowTLSBin))
		if len(steps) > 0 && steps[len(steps)-1].Err != nil {
			return steps
		}
	}

	return steps
}

func downloadComponent(ctx context.Context, comp core.Component, destPath string) DepStep {
	step := DepStep{Description: fmt.Sprintf("下载 %s 二进制", comp)}

	if !core.HasRepo(comp) {
		step.Err = fmt.Errorf("no download source for %s", comp)
		return step
	}

	check, err := core.CheckUpdate(ctx, comp, destPath)
	if err != nil {
		step.Err = fmt.Errorf("check latest version: %w", err)
		return step
	}

	url := check.DownloadURL
	if url == "" {
		// If no update check diff (new install), get URL from latest release directly.
		url = check.DownloadURL
	}
	if url == "" {
		step.Err = fmt.Errorf("no download URL found for %s", comp)
		return step
	}

	if err := core.DownloadBinary(ctx, url, destPath); err != nil {
		step.Err = err
		return step
	}

	step.Description = fmt.Sprintf("下载 %s %s", comp, check.LatestVersion)
	return step
}

func enableAndStart(ctx context.Context, svc service.Name) []DepStep {
	var steps []DepStep

	step := DepStep{Description: fmt.Sprintf("启用 %s 服务", svc)}
	if err := service.Enable(ctx, svc); err != nil {
		step.Err = err
	}
	steps = append(steps, step)

	step = DepStep{Description: fmt.Sprintf("启动 %s 服务", svc)}
	if err := service.Start(ctx, svc); err != nil {
		step.Err = err
	}
	steps = append(steps, step)

	return steps
}

// HasDepError checks if any step in the list has an error.
func HasDepError(steps []DepStep) bool {
	for _, s := range steps {
		if s.Err != nil {
			return true
		}
	}
	return false
}

// FormatDepSteps formats dependency steps for display.
func FormatDepSteps(steps []DepStep) string {
	if len(steps) == 0 {
		return ""
	}
	var result string
	for _, s := range steps {
		if s.Err != nil {
			result += fmt.Sprintf("  ✗ %s: %s\n", s.Description, s.Err)
		} else {
			result += fmt.Sprintf("  ✓ %s\n", s.Description)
		}
	}
	return result
}
