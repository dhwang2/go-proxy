package sysutil

import (
	"bufio"
	"net"
	"os"
	"runtime"
	"strings"
)

type PlatformInfo struct {
	OS      string
	Arch    string
	Kernel  string
	IPStack string
}

func DetectPlatform() PlatformInfo {
	return PlatformInfo{
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
		Kernel:  detectKernel(),
		IPStack: DetectIPStack(),
	}
}

func detectKernel() string {
	f, err := os.Open("/proc/version")
	if err != nil {
		return "unknown"
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	if s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line != "" {
			return line
		}
	}
	return "unknown"
}

func DetectIPStack() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "unknown"
	}
	has4 := false
	has6 := false
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil {
				continue
			}
			if ip.To4() != nil {
				has4 = true
			} else if ip.To16() != nil {
				has6 = true
			}
		}
	}
	switch {
	case has4 && has6:
		return "dual-stack"
	case has4:
		return "ipv4"
	case has6:
		return "ipv6"
	default:
		return "unknown"
	}
}
