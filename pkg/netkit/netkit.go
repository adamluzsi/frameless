package netkit

import (
	"errors"
	"fmt"
	"go.llib.dev/frameless/pkg/enum"
	"io"
	"net"
	"strconv"
	"strings"
)

type Network string

const (
	TCP  Network = "tcp"
	TCP4 Network = "tcp4"
	TCP6 Network = "tcp6"
)

const (
	UDP  Network = "udp"
	UDP4 Network = "udp4"
	UDP6 Network = "udp6"
)

var _ = enum.Register[Network]("",
	TCP, TCP4, TCP6,
	UDP, UDP4, UDP6,
)

// IsPortFree checks if a given TCP port is free to bind to. It takes into account specifics of
//
// MacOS behavior where multiple IP versions (IPv4 and IPv6) may be bound to the same port,
// which can lead to false positives in port availability checking.
// IsPortFree able to work this around by checking both ip versions for the port,
// to give a unified behaviour across different platforms.
//
// If you wish to check both TCP and UDP networks, then give a zero value to the Network argument.
func IsPortFree(network Network, port int) (ok bool, returnErr error) {
	if err := enum.Validate[Network](network); err != nil {
		return false, err
	}
	const (
		minPort = 0
		maxPort = 65535
	)
	if port < minPort || maxPort < port {
		return false, nil
	}
	expandNetwork := func(n Network) []Network {
		switch n {
		case TCP:
			return []Network{TCP, TCP4, TCP6}
		case UDP:
			return []Network{UDP, UDP4, UDP6}
		case "": // scan all network type
			return []Network{TCP, TCP4, TCP6, UDP, UDP4, UDP6}
		default:
			return []Network{n}
		}
	}
	ip := net.ParseIP("0.0.0.0") // bind to all available interfaces
	for _, n := range expandNetwork(network) {
		var (
			c   io.Closer
			err error
		)
		switch {
		case isUDP(n):
			c, err = net.ListenUDP(string(n), &net.UDPAddr{
				IP:   ip,
				Port: port,
			})
		case isTCP(n):
			c, err = net.ListenTCP(string(n), &net.TCPAddr{
				IP:   ip,
				Port: port,
			})
		default:
			panic(fmt.Errorf("unknown network type: %#v", n))
		}
		if err == nil {
			_ = c.Close()
			continue
		}
		var opErr *net.OpError
		if errors.As(err, &opErr) && opErr.Op == "listen" &&
			strings.Contains(err.Error(), "address already in use") {
			return false, nil
		} else {
			return false, err
		}
	}
	return true, nil
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

func isUDP(n Network) bool {
	return strings.HasPrefix(string(n), string(UDP))
}

func isTCP(n Network) bool {
	return strings.HasPrefix(string(n), string(TCP))
}
