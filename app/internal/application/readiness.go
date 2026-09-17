package application

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"go-proxy/internal/service"
)

func verifyListeners(ctx context.Context, snapshot *Snapshot, name service.Name) error {
	wanted := map[string]map[int]bool{"tcp": {}, "udp": {}}
	if name == service.SingBox {
		for _, ib := range snapshot.Store.SingBox.Inbounds {
			if ib.Type == "tuic" {
				wanted["udp"][ib.ListenPort] = true
			} else {
				wanted["tcp"][ib.ListenPort] = true
			}
			if ib.Type == "shadowsocks" {
				wanted["udp"][ib.ListenPort] = true
			}
		}
	} else if name == service.Snell && snapshot.Store.SnellConf != nil {
		wanted["tcp"][snapshot.Store.SnellConf.Port()] = true
	}
	if len(wanted["tcp"])+len(wanted["udp"]) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		found := map[string]map[int]bool{"tcp": {}, "udp": {}}
		for _, table := range []string{"tcp", "tcp6", "udp", "udp6"} {
			file, err := os.Open("/proc/net/" + table)
			if os.IsNotExist(err) && strings.HasSuffix(table, "6") {
				continue
			}
			if err != nil {
				return fmt.Errorf("inspect listeners: %w", err)
			}
			transport := strings.TrimSuffix(table, "6")
			scan := bufio.NewScanner(file)
			for scan.Scan() {
				if err := ctx.Err(); err != nil {
					file.Close()
					return err
				}
				fields := strings.Fields(scan.Text())
				if len(fields) < 4 {
					continue
				}
				if transport == "tcp" && fields[3] != "0A" {
					continue
				}
				_, portText, ok := strings.Cut(fields[1], ":")
				if !ok {
					continue
				}
				port, err := strconv.ParseInt(portText, 16, 32)
				if err == nil && wanted[transport][int(port)] {
					found[transport][int(port)] = true
				}
			}
			err = scan.Err()
			file.Close()
			if err != nil {
				return err
			}
		}
		ready := true
		for transport, ports := range wanted {
			for port := range ports {
				if !found[transport][port] {
					ready = false
				}
			}
		}
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s listeners did not become ready: %w", name, ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}
