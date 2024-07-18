package main

import (
	"errors"
	"net"
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
