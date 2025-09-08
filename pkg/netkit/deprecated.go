package netkit

// GetFreePort returns a
//
// Deprecated: use netkit.FreePort("tcp") instead
func GetFreePort() (int, error) {
	return FreePort(TCP)
}
