package main

import (
	"crypto/tls"
	"errors"
	"fmt"
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

type ErrorCause struct {
	error
	Cause string
}

func (c *ErrorCause) Error() string {
	return fmt.Sprintf("cause %s: %s", c.Cause, c.error.Error())
}

func (c *ErrorCause) Unwrap() error {
	return c.error
}

type BackendError struct {
	error
	Backend string
}

func (b *BackendError) Error() string {
	return fmt.Sprintf("backend %s: %s", b.Backend, b.error.Error())
}

func (b *BackendError) Unwrap() error {
	return b.error
}

func errorLabelValue(err error) string {
	var edb *DisallowedBackend
	var rhe tls.RecordHeaderError

	switch {
	case errors.Is(err, ErrNoSNI):
		return "no-sni"
	case errors.Is(err, io.EOF):
		return "eof"
	case errors.Is(err, os.ErrDeadlineExceeded):
		return "timeout"
	case errors.Is(err, syscall.ECONNRESET):
		return "connection-reset"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection-refused"
	case errors.Is(err, syscall.ENETUNREACH):
		return "network-unreachable"
	case errors.Is(err, syscall.EHOSTUNREACH):
		return "no-route-to-host"
	case errors.As(err, &edb):
		return "disallowed-backend"
	case errors.As(err, &rhe):
		return "tls-invalid"
	default:
		return "unknown"
	}
}
