//go:build windows

package session

import "strings"

// SameUser compares two account names. Windows account names are
// case-insensitive.
func SameUser(a, b string) bool {
	return strings.EqualFold(a, b)
}
