// Copyright (C) 2022 Andrew Ayer
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the "Software"),
// to deal in the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
// OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
// OTHER DEALINGS IN THE SOFTWARE.
//
// Except as contained in this notice, the name(s) of the above copyright
// holders shall not be used in advertising or otherwise to promote the
// sale, use or other dealings in this Software without prior written
// authorization.

package main

import (
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"src.agwa.name/go-listener/proxy"
	"src.agwa.name/go-listener/tlsutil"
)

type Server struct {
	Backend         BackendDialer
	ProxyProtocol   bool
	DefaultHostname string

	metrics ServerCollector
}

func (server *Server) peekClientHello(clientConn net.Conn) (*tls.ClientHelloInfo, net.Conn, error) {
	if err := clientConn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return nil, nil, err
	}

	clientHello, peekedClientConn, err := tlsutil.PeekClientHelloFromConn(clientConn)
	if err != nil {
		return nil, nil, err
	}

	if err := clientConn.SetReadDeadline(time.Time{}); err != nil {
		return nil, nil, err
	}

	if clientHello.ServerName == "" {
		if server.DefaultHostname == "" {
			return nil, nil, ErrNoSNI
		}
		clientHello.ServerName = server.DefaultHostname
	}

	return clientHello, peekedClientConn, err
}

func (server *Server) handleConnection(clientConn net.Conn) error {
	defer clientConn.Close()

	var clientHello *tls.ClientHelloInfo

	if peekedClientHello, peekedClientConn, err := server.peekClientHello(clientConn); err == nil {
		clientHello = peekedClientHello
		clientConn = peekedClientConn
	} else {
		// Ignore client EOF/timeout errors as they're almost certainly
		// scanners closing the connection immediately
		if !errors.Is(err, io.EOF) && !os.IsTimeout(err) {
			log.Printf("Peeking client hello from %s failed: %s", clientConn.RemoteAddr(), err)
			return nil
		}

		return err
	}

	start := time.Now()
	backendConn, err := server.Backend.Dial(clientHello.ServerName, clientHello.SupportedProtos, clientConn)
	if err != nil {
		log.Printf("Ignoring connection from %s to %s because dialing backend failed: %s", clientConn.RemoteAddr(), clientHello.ServerName, err)
		return err
	}
	defer backendConn.Close()
	dialTime := time.Since(start)
	server.metrics.setupTime.Observe(dialTime.Seconds())

	if server.ProxyProtocol {
		header := proxy.Header{RemoteAddr: clientConn.RemoteAddr(), LocalAddr: clientConn.LocalAddr()}
		if _, err := backendConn.Write(header.Format()); err != nil {
			log.Printf("Error writing PROXY header to backend: %s", err)
			return err
		}
	}

	go func() {
		io.Copy(backendConn, clientConn)
		backendConn.CloseWrite()
	}()

	io.Copy(clientConn, backendConn)
	return nil
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
	default:
		return "unknown"
	}
}

func (server *Server) Serve(listener net.Listener) error {
	labels := prometheus.Labels{"listener": listener.Addr().String()}
	connCount := server.metrics.connCount.With(labels)
	errCount := server.metrics.connErrors.MustCurryWith(labels)
	inflight := server.metrics.inflight.With(labels)
	for {
		conn, err := listener.Accept()
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Temporary() {
				log.Printf("Temporary network error accepting connection: %s", netErr)
				errCount.With(prometheus.Labels{"error": "transient"}).Inc()
				continue
			}
			return err
		}

		// Instrument bytes read/written to/from client connections
		conn = InstrumentedConn(conn, server.metrics.clientReadBytes, server.metrics.clientWriteBytes)

		go func(conn net.Conn) {
			connCount.Inc()
			inflight.Inc()
			err := server.handleConnection(conn)
			if err != nil {
				errCount.With(prometheus.Labels{"error": errorLabelValue(err)}).Inc()
			}
			inflight.Dec()
		}(conn)
	}
}
