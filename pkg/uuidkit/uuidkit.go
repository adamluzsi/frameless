// Package uuidkit holds utilities for the standard library's uuid.UUID
// that the standard library itself doesn't provide.
//
// Everything here works with the plain stdlib uuid.UUID type,
// so nothing in this package ties your code to go.llib.dev/frameless/pkg/uuid.
// That package only remains as a backward compatibility layer.
package uuidkit

import "uuid"

// Version returns the UUID version (e.g. 4 or 7),
// read from the high nibble of the 7th byte.
//
// It replaces the former pkg/uuid.UUID#Version method,
// since the standard library's uuid.UUID has no such method.
func Version(u uuid.UUID) int {
	return int(u[6] >> 4)
}

// Variant returns the UUID variant,
// read from the two most significant bits of the 9th byte.
// UUIDs following RFC 9562 report 2 (0b10).
//
// It replaces the former pkg/uuid.UUID#Variant method,
// since the standard library's uuid.UUID has no such method.
func Variant(u uuid.UUID) int {
	return int(u[8] >> 6)
}
