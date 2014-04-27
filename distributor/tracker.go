/*
 * tracker.go
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

type Tracker struct {
	listener net.Listener
}

func (self *Tracker) handler() {
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

func setupTracker(ip string, port int) *Tracker {
	addr, err := net.ResolveTCPAddr("tcp4", fmt.Sprintf("%s:%d", ip, port))
	if err != nil {
		logfatal("Resolving address: %s", err)
	}

	listener, err := net.ListenTCP("tcp4", addr)
	if err != nil {
		logfatal("Listening: %s", err)
	}

	tracker := &Tracker{listener: listener}
	go tracker.handler()
	return tracker
}
