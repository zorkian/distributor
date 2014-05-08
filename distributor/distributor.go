/*
 * distributor.go
 *
 * Program for helping with distributing files from a directory using BitTorrent.
 *
 * Copyright (c) 2014 by authors and contributors. Please see the included LICENSE file for
 * licensing information.
 *
 */

package main

import (
	"flag"
	"os"
	"path/filepath"
)

// 2 = debug, 1 = verbose, 0 = normal
var VERBOSITY uint32 = 0

func main() {
	verbose := flag.Bool("verbose", false, "Verbose mode (extra output)")
	debug := flag.Bool("debug", false, "Extra verbose (debugging output)")
	listen := flag.String("listen", "127.0.0.1", "IP address to bind to for serving")
	port := flag.Int("port", 6390, "Port to serve tracker/torrents on")
	dir := flag.String("serve", "/var/www", "Directory to serve files from")
	flag.Parse()

	info, err := os.Stat(*dir)
	if err != nil {
		logfatal("-serve does not exist: %s", err)
	}
	if !info.IsDir() {
		logfatal("-serve is not a directory")
	}
	*dir = filepath.Clean(*dir) // Canonicalize.

	if *debug {
		VERBOSITY = 2
	} else if *verbose {
		VERBOSITY = 1
	}

	if *port < 1 || *port > 65535 {
		logfatal("-port must be in range 1..65535")
	}

	// The basic flow is that we set up a tracker, which listens on a port for HTTP requests. The
	// tracker coordinates peers and torrent files. To each tracker we can attach a watcher, which
	// handles monitoring of files.

	doQuit := make(chan bool)
	watchers := []*Watcher{
		startWatcher(*dir),
	}
	startTracker(*listen, *port, watchers)

	loginfo("distributing %s on %s:%d", *dir, *listen, *port)
	<-doQuit
}
