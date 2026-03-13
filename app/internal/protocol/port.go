package protocol

import "github.com/dhwang2/go-proxy/pkg/sysutil"

var portAvailableFn = sysutil.IsTCPPortAvailable

func PortAvailable(port int) bool {
	return portAvailableFn(port)
}
