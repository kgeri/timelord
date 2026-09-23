//go:build !linux

package process

import "errors"

// ErrNotImplemented is returned when the operating system has no process lister.
var ErrNotImplemented = errors.New("process listing is not implemented on this operating system")

// unsupportedLister is used on operating systems without a real implementation.
type unsupportedLister struct{}

// NewLister returns a lister that reports the missing implementation.
func NewLister() Lister {
	return &unsupportedLister{}
}

func (*unsupportedLister) List() ([]Process, error) {
	return nil, ErrNotImplemented
}
