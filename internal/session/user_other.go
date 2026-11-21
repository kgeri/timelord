//go:build !windows

package session

// SameUser compares two account names. Non-Windows account names are
// case-sensitive.
func SameUser(a, b string) bool {
	return a == b
}
