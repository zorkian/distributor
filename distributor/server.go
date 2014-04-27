/*
 * server.go
 *
 * This is the HTTP server implementation that automatically generates torrent files for the
 * files that we're serving.
 *
 * Copyright (c) 2014 by authors and contributors. Please see the included LICENSE file for
 * licensing information.
 *
 */

package main

import (
	"fmt"
	"net"
)

type Server struct {
	listener net.Listener
}

func (self *Server) handler() {
	for {
		client, err := self.listener.Accept()
		if err != nil {
			// TODO: handle failures to listen (which can be transient)
			logfatal("Failed to accept: %s", err)
		}

		wrapped_client := &Client{Conn: client}
		go wrapped_client.Handler()
	}
}

func setupServer(ip string, port int) *Server {
	addr, err := net.ResolveTCPAddr("tcp4", fmt.Sprintf("%s:%d", ip, port))
	if err != nil {
		logfatal("Resolving address: %s", err)
	}

	listener, err := net.ListenTCP("tcp4", addr)
	if err != nil {
		logfatal("Listening: %s", err)
	}

	server := &Server{listener: listener}
	go server.handler()
	return server
}
