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

func (server *Server) handleConnection(clientConn net.Conn, listenAddr string) (err error) {
	defer func() {
		if err != nil {
			// Use SetLinger to send a RST instead of FIN
			conn, ok := clientConn.(*net.TCPConn)
			if ok {
				conn.SetLinger(0)
			}
		}
		clientConn.Close()
	}()

	var clientHello *tls.ClientHelloInfo
	defer func() {
		if err != nil && clientHello != nil {
			// If we got as far as identifying the backend, wrap the error to propagate the backend information upwards
			err = &BackendError{
				error:   err,
				Backend: clientHello.ServerName,
			}
		}
	}()

	clientHello, peekedClientConn, err := server.peekClientHello(clientConn)
	if err != nil {
		// Ignore client EOF/timeout errors as they're almost certainly
		// scanners closing the connection immediately
		if !errors.Is(err, io.EOF) && !os.IsTimeout(err) {
			log.Printf("Peeking client hello from %s failed: %s", clientConn.RemoteAddr(), err)
		}
		return &ErrorCause{err, "client"}
	}
	clientConn = peekedClientConn

	backend := clientHello.ServerName
	if parsed := net.ParseIP(backend); parsed != nil {
		log.Printf("Ignoring connection from %s to %s because SNI is an IP address", clientConn.RemoteAddr(), backend)
		err = &DisallowedBackend{Backend: parsed}
		return &ErrorCause{err, "client"}
	}

	labels := prometheus.Labels{"listener": listenAddr, "backend": backend}
	server.metrics.beConnCount.With(labels).Inc()

	start := time.Now()
	backendConn, err := server.Backend.Dial(backend, clientHello.SupportedProtos, clientConn)
	if err != nil {
		log.Printf("Ignoring connection from %s to %s because dialing backend failed: %s", clientConn.RemoteAddr(), backend, err)
		cause := "backend"
		// Disallowed backend errors are client errors, not backend errors
		var dbe *DisallowedBackend
		if errors.As(err, &dbe) {
			cause = "client"
		}
		return &ErrorCause{err, cause}
	}
	defer backendConn.Close()
	dialTime := time.Since(start)
	server.metrics.beSetupTime.With(labels).Observe(dialTime.Seconds())

	if server.ProxyProtocol {
		header := proxy.Header{RemoteAddr: clientConn.RemoteAddr(), LocalAddr: clientConn.LocalAddr()}
		if _, err := backendConn.Write(header.Format()); err != nil {
			log.Printf("Error writing PROXY header to backend: %s", err)
			return err
		}
	}

	// Instrument bytes to/from client connection
	// Note that read/write are flipped because reading from the client is
	// counted as writing to the backend. Instrumenting this could be done
	// either way around, but this was easier
	clientConn = InstrumentedConn(clientConn, server.metrics.beWriteBytes.With(labels), server.metrics.beReadBytes.With(labels))

	go func() {
		io.Copy(backendConn, clientConn)
		backendConn.CloseWrite()
	}()

	io.Copy(clientConn, backendConn)
	return nil
}

func (server *Server) Serve(listener net.Listener) error {
	listenAddr := listener.Addr().String()
	labels := prometheus.Labels{"listener": listenAddr}
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

		go func(conn net.Conn) {
			connCount.Inc()
			inflight.Inc()
			err := server.handleConnection(conn, listenAddr)
			if err != nil {
				var ec *ErrorCause = &ErrorCause{Cause: "unknown"}
				var be *BackendError = &BackendError{Backend: ""}
				errors.As(err, &ec)
				errors.As(err, &be)
				labels := prometheus.Labels{
					"error":   errorLabelValue(err),
					"cause":   ec.Cause,
					"backend": be.Backend,
				}
				errCount.With(labels).Inc()
			}
			inflight.Dec()
		}(conn)
	}
}
