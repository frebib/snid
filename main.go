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
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	"src.agwa.name/go-listener"
)

var (
	stopping = false
)

func main() {
	var flags struct {
		listen          []string
		defaultHostname string
		mode            string
		proxyProto      bool
		unixDirectory   string
		backendCidr     []*net.IPNet
		backendPort     int
		nat46Prefix     net.IP
		addRoute        bool
	}
	flag.Func("listen", "Socket to listen on (repeatable)", func(arg string) error {
		flags.listen = append(flags.listen, arg)
		return nil
	})
	flag.StringVar(&flags.defaultHostname, "default-hostname", "", "Default hostname if client does not provide SNI")
	flag.StringVar(&flags.mode, "mode", "", "unix, tcp, or nat46")
	flag.BoolVar(&flags.proxyProto, "proxy-proto", false, "Use PROXY protocol when talking to backend (tcp, unix modes)")
	flag.StringVar(&flags.unixDirectory, "unix-directory", "", "Path to directory containing backend UNIX sockets (unix mode)")
	flag.Func("backend-cidr", "CIDR of allowed backends (repeatable) (tcp, nat46 modes)", func(arg string) error {
		_, ipnet, err := net.ParseCIDR(arg)
		if err != nil {
			return err
		}
		flags.backendCidr = append(flags.backendCidr, ipnet)
		return nil
	})
	flag.IntVar(&flags.backendPort, "backend-port", 0, "Port number of backend (defaults to same port number as listener) (tcp mode)")
	flag.Func("nat46-prefix", "IPv6 prefix for NAT46 source address (nat46 mode)", func(arg string) error {
		flags.nat46Prefix = net.ParseIP(arg)
		if flags.nat46Prefix == nil {
			return fmt.Errorf("not a valid IP address")
		}
		if flags.nat46Prefix.To4() != nil {
			return fmt.Errorf("not an IPv6 address")
		}
		return nil
	})
	flag.BoolVar(&flags.addRoute, "add-local-route", false, "Insert route for nat46-prefix into the local routing table (nat46 mode)")
	flag.Parse()

	server := &Server{
		ProxyProtocol:   flags.proxyProto,
		DefaultHostname: flags.defaultHostname,
	}

	switch flags.mode {
	case "unix":
		if flags.unixDirectory == "" {
			log.Fatal("-unix-directory must be specified when you use -mode unix")
		}
		server.Backend = &UnixDialer{Directory: flags.unixDirectory}
	case "tcp":
		if len(flags.backendCidr) == 0 {
			log.Fatal("At least one -backend-cidr flag must be specified when you use -mode tcp")
		}
		server.Backend = &TCPDialer{Port: flags.backendPort, Allowed: flags.backendCidr}
	case "nat46":
		if flags.proxyProto {
			log.Fatal("-proxy-proto must not be specified when you use -mode nat46")
		}
		if flags.backendPort != 0 {
			log.Fatal("-backend-port must not be specified when you use -mode nat46")
		}
		if len(flags.backendCidr) == 0 {
			log.Fatal("At least one -backend-cidr flag must be specified when you use -mode nat46")
		}
		if flags.nat46Prefix == nil {
			log.Fatal("-nat46-prefix must be specified when you use -mode nat46")
		}
		server.Backend = &TCPDialer{
			Allowed:          flags.backendCidr,
			IPv6SourcePrefix: flags.nat46Prefix,
		}

		if flags.addRoute {
			lo, err := netlink.LinkByName("lo")
			if err != nil {
				log.Fatalf("Could not find loopback interface: %s", err)
			}
			r := netlink.Route{
				LinkIndex: lo.Attrs().Index,
				Type:      unix.RTN_LOCAL,
				Dst: &net.IPNet{
					IP:   flags.nat46Prefix,
					Mask: net.CIDRMask(96, 128),
				},
				Table: 255, // 255 is local table
			}
			err = netlink.RouteAdd(&r)
			if err != nil && !errors.Is(err, os.ErrExist) {
				log.Fatalf("Failed to add route: %s", err)
			}
			defer func() {
				err = netlink.RouteDel(&r)
				if err != nil && !errors.Is(err, syscall.ESRCH) {
					log.Printf("Failed to remove route: %s", err)
				}
			}()
		}
	default:
		log.Fatal("-mode must be unix, tcp, or nat46")
	}

	if len(flags.listen) == 0 {
		log.Fatal("At least one -listen flag must be specified")
	}

	listeners, err := listener.OpenAll(flags.listen)
	if err != nil {
		log.Fatal(err)
	}
	defer listener.CloseAll(listeners)

	for _, l := range listeners {
		go serve(l, server)
	}

	// Wait for termination signal and exit cleanly
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	stopping = true
}

func serve(listener net.Listener, server *Server) {
	err := server.Serve(listener)
	if nil != err && !errors.Is(err, net.ErrClosed) {
		if stopping {
			log.Print(err)
		} else {
			log.Fatal(err)
		}
	}
}
