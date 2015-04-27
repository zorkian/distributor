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
	"distributor/torrent"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
)

var CTORRENT string = "/usr/local/bin/ctorrent"

func main() {
	verbose := flag.Bool("verbose", false, "Verbose mode (extra output)")
	debug := flag.Bool("debug", false, "Extra verbose (debugging output)")
	listen := flag.String("listen", "127.0.0.1", "IP address to bind to for serving")
	port := flag.Int("port", 6390, "Port to serve tracker/torrents on")
	dir := flag.String("serve", "/var/www", "Directory to serve files from")
	ctorrent := flag.String("ctorrent", CTORRENT, "Path to ctorrent binary")
	flag.Parse()

	info, err := os.Stat(*dir)
	if err != nil {
		torrent.LogFatal("-serve does not exist: %s", err)
	}
	if !info.IsDir() {
		torrent.LogFatal("-serve is not a directory")
	}
	*dir = filepath.Clean(*dir) // Canonicalize.
	verbosity := torrent.VerbNormal
	if *debug {
		verbosity = torrent.VerbDebug
	} else if *verbose {
		verbosity = torrent.VerbVerbose
	}
	torrent.SetLoggingVerbosity(verbosity)

	distributor, err := torrent.NewDistributor(*dir, *ctorrent, *listen, *port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error Creating distributor: %v\n", err)
		os.Exit(1)
	}
	go distributor.Run()

	// Make the system keep running until it receives an interrupt or kill signal
	// from the OS; then cleanup and exit.
	doQuit := make(chan os.Signal)
	signal.Notify(doQuit, os.Interrupt, os.Kill)
	_ = <-doQuit
	distributor.Close()
	os.Exit(0)
}
