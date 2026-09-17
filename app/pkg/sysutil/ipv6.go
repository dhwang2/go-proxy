package sysutil

import "net"

func IPv6Available() bool {
	listener, err := net.Listen("tcp6", "[::]:0")
	if err != nil {
		return false
	}
	return listener.Close() == nil
}
