package netkit

import (
	"errors"
	"net"
	"strconv"
)

// IsPortFree checks if a port is open.
func IsPortFree(port int) (bool, error) {
	listener, err := net.Listen("tcp", "0.0.0.0:"+strconv.Itoa(port))
	if err != nil {
		var opErr *net.OpError
		if errors.As(err, &opErr) && opErr.Op == "listen" {
			return false, nil
		}
		return false, err
	}
	return true, listener.Close()
}

// GetFreePort opens a TCP listener on a randomly selected available port.
// It returns the listener and the selected port number.
func GetFreePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}

	// Determine the port selected by the operating system.
	addr := listener.Addr().String()
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, err
	}
	if err := listener.Close(); err != nil {
		return 0, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, err
	}
	return port, nil
}
