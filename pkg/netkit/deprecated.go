package netkit

// GetFreePort returns a TCP port number which is free to start listening on.
//
// Deprecated: use netkit.FreePort("tcp") instead
func GetFreePort() (int, error) {
	return FreePort(TCP)
}
