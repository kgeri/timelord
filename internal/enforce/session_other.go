//go:build !linux

package enforce

import "errors"

// NewSessionSource reports that session enforcement is not implemented on this
// platform.
func NewSessionSource() (SessionSource, error) {
	return nil, errors.New("session enforcement is only supported on Linux")
}
