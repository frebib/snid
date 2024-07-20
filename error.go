package main

import (
	"errors"
	"io"
	"net"
	"os"
	"syscall"
)

var (
	ErrNoSNI = errors.New("no SNI provided and DefaultHostname not set")
)

type DisallowedBackend struct {
	Backend net.IP
}

func (e *DisallowedBackend) Error() string {
	return "disallowed backend: " + e.Backend.String()
}

func errorLabelValue(err error) string {
	var edb *DisallowedBackend

	switch {
	case os.IsTimeout(err):
		return "timeout"
	case errors.Is(err, ErrNoSNI):
		return "no-sni"
	case errors.Is(err, io.EOF):
		return "eof"
	case errors.As(err, &edb):
		return "disallowed-backend"
	case errors.Is(err, syscall.ENETUNREACH):
		return "network-unreachable"
	case errors.Is(err, syscall.EHOSTUNREACH):
		return "no-route-to-host"
	default:
		return "unknown"
	}
}
