/*
 * client.go
 *
 * Copyright (c) 2014 by authors and contributors. Please see the included LICENSE file for
 * licensing information.
 *
 */

package main

import (
	"bufio"
	"net"
)

type Client struct {
	Conn    net.Conn
	Address string
}

// Handler is the main loop that reads input from the clients. For simplicity sake, the protocol
// is defined such that clients are sending newline-buffered commands.
func (self *Client) Handler() {
	// One-time setup
	self.Address = self.Conn.RemoteAddr().String()
	logdebug("Connection from %s", self.Address)

	// Line reading protocol
	buf := bufio.NewReader(self.Conn)
	for {
		line, err := buf.ReadSlice('\n')
		if err != nil {
			self.Conn.Close()
			logerror("Closing ")
			return
		}

		if len(line) <= 1 {
			continue
		}

		cmd := line[0 : len(line)-1]
		logwarning("got [%s]", cmd)
	}
}
