package netkit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/resilience"
)

type Network string

func (n Network) String() string { return string(n) }

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

var _ = enum.Register[Network](
	TCP, TCP4, TCP6,
	UDP, UDP4, UDP6,
)

var allAvailableInterfaces = net.ParseIP("0.0.0.0") // bind to all available interfaces
var localhost = net.ParseIP("127.0.0.1")            // bind to all available interfaces

var anyNetworkIsPortFree = map[Network]struct{}{
	"*": {},
	"":  {},
}

// IsPortFree checks if a given TCP port is free to bind to. It takes into account specifics of
//
// MacOS behavior where multiple IP versions (IPv4 and IPv6) may be bound to the same port,
// which can lead to false positives in port availability checking.
// IsPortFree able to work this around by checking both ip versions for the port,
// to give a unified behaviour across different platforms.
//
// If you wish to check both TCP and UDP networks, then give a zero value to the Network argument.
func IsPortFree(network Network, port int) (ok bool, returnErr error) {
	if _, ok := anyNetworkIsPortFree[network]; ok {
		network = "*"
	} else if err := enum.Validate(network); err != nil {
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
		case "*": // scan all network type
			return []Network{TCP, TCP4, TCP6, UDP, UDP4, UDP6}
		default:
			return []Network{n}
		}
	}
	for _, ip := range []net.IP{allAvailableInterfaces, localhost} {
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

			if opErr, ok := errorkit.As[*net.OpError](err); ok && opErr.Op == "listen" {
				if scErr, ok := errorkit.As[*os.SyscallError](opErr.Err); ok {
					if errors.Is(scErr.Err, syscall.EADDRINUSE) {
						return false, nil
					}
					if errors.Is(scErr.Err, syscall.EADDRNOTAVAIL) {
						continue
					}
				}
			}

			return false, err
		}
	}
	return true, nil
}

// FreePort attempts to request from the OS an available local free port for a given network type.
//
// Be careful, as it might take a bit time to have the port fully released before you can use it to initiate a new connection
func FreePort(n Network) (int, error) {
	if err := enum.Validate(n); err != nil {
		return 0, err
	}
	if isUDP(n) {
		pc, err := net.ListenPacket(n.String(), ":0")
		if err != nil {
			return 0, err
		}
		if err := pc.Close(); err != nil {
			return 0, err
		}
		return toFreePort(n, pc.LocalAddr().String())
	}
	listener, err := net.Listen(n.String(), ":0")
	if err != nil {
		return 0, err
	}
	if err := listener.Close(); err != nil {
		return 0, err
	}
	return toFreePort(n, listener.Addr().String())
}

func toFreePort(n Network, addr string) (int, error) {
	_, portEnc, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, err
	}
	port, err := strconv.Atoi(portEnc)
	if err != nil {
		return 0, err
	}
	w := resilience.Waiter{Timeout: time.Second, WaitDuration: time.Millisecond}
	for range resilience.Retries(context.Background(), w) {
		conn, err := net.DialTimeout(n.String(), addr, time.Millisecond)
		if err != nil {
			break
		}
		_ = conn.Close()
	}
	return port, nil // still listening
}

func isUDP(n Network) bool {
	return strings.HasPrefix(string(n), string(UDP))
}

func isTCP(n Network) bool {
	return strings.HasPrefix(string(n), string(TCP))
}
