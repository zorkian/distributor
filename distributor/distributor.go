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

import ()

// 2 = debug, 1 = verbose, 0 = normal
var verbose uint32 = 2

func main() {
	// TODO add flags for setting port, directory, etc

	doQuit := make(chan bool)

	// The basic flow is that we set up a tracker, which listens on a port for HTTP requests. The
	// tracker coordinates peers and torrent files. To each tracker we can attach a watcher, which
	// handles monitoring of files.

	watchers := []*Watcher{
		startWatcher("/Users/mark/torrents"),
	}
	startTracker("127.0.0.1", 6969, watchers)

	<-doQuit
	loginfo("distributor listening")
}
