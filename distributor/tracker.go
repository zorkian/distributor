/*
 * tracker.go
 *
 * The tracker code pulls double duty as both our tracker (helps peers find each other) but
 * is also the endpoint where people download torrent files.
 *
 * Copyright (c) 2014 by authors and contributors. Please see the included LICENSE file for
 * licensing information.
 *
 */

package main

import (
	bencode "code.google.com/p/bencode-go"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"
)

type Tracker struct {
	AnnounceURL string // Points back at our HTTP server.

	// Careful: there is no locking here. It's assumed that the only time this is
	// written is from the very initial setup of the app and never during runtime. If that
	// changes we'll need locking. (This may actually be technically a little racy right
	// now if there's a ton of requests during power-on, since we start listening
	// before the watchers are created.)
	watchers []*Watcher // List of watchers who might have files.
}

func (self *Tracker) findFile(name string) *File {
	for _, watcher := range self.watchers {
		file := watcher.GetFile(name)
		if file != nil {
			return file
		}
	}
	return nil
}

func (self *Tracker) handleServe(w http.ResponseWriter, r *http.Request) {
	pieces := strings.SplitN(r.URL.RequestURI(), "?", 2)
	if len(pieces) != 2 {
		io.WriteString(w, "invalid request")
		return
	}

	file := self.findFile(pieces[1])
	if file == nil {
		io.WriteString(w, "file not found")
		return
	}

	for {
		// TODO: This could run infinitely in a case where the file is requested and deleted or
		// replaced, so we keep checking a structure that never will get filled in since it's no
		// longer active.
		if file.MetadataInfo == nil {
			logdebug("Request for missing metadata on %s. Sleeping.", file.Name)
			time.Sleep(1 * time.Second)
			continue
		}
		break
	}

	md := Metadata{
		Announce: self.AnnounceURL,
		Info:     *file.MetadataInfo,
	}

	err := bencode.Marshal(w, md)
	if err != nil {
		logerror("Failed to bencode %s: %s", file.Name, err)
	}
}

func (self *Tracker) handleAnnounce(w http.ResponseWriter, r *http.Request) {
	loginfo("Requested /announce: %s", r.URL.RequestURI())
	fmt.Fprintf(w, "Hello, %q", html.EscapeString(r.URL.RequestURI()))
}

func startTracker(ip string, port int, watchers []*Watcher) *Tracker {
	tracker := &Tracker{
		AnnounceURL: fmt.Sprintf("http://%s:%d/announce", ip, port),
		watchers:    watchers,
	}

	http.HandleFunc("/serve", tracker.handleServe)
	http.HandleFunc("/announce", tracker.handleAnnounce)

	go func() {
		err := http.ListenAndServe(fmt.Sprintf("%s:%d", ip, port), nil)
		logfatal("HTTP server exited: %s", err)
	}()

	return tracker
}
