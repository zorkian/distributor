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
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Peer struct {
	Id   string `peer id`
	Ip   string `ip`
	Port uint16 `port`
}

type PeerResponse struct {
	Interval int    `interval`
	Peers    []Peer `peers`
}

type Tracker struct {
	AnnounceURL string // Points back at ourselves.

	// We keep a separate set of peers for each info_hash. We don't actually verify that these
	// hashes are valid; so there's a pretty easy DoS here. This system is designed to be used
	// in a production environment with good actors. TODO: harden.
	// TODO: We need a way of droppign peers that have not reported in a while.
	PeerList     map[string]map[string]Peer
	peerListLock sync.Mutex

	// Lock used by all methods that affect the seed process.
	seedStartLock sync.Mutex

	// Careful: there is no locking here. It's assumed that the only time this is
	// written is from the very initial setup of the app and never during runtime. If that
	// changes we'll need locking. (This may actually be technically a little racy right
	// now if there's a ton of requests during power-on, since we start listening
	// before the watchers are created.)
	watchers []*Watcher // List of watchers who might have files.
}

// findFile searches all of our watchers for a given filename (FQFN). If found, it returns
// the pointer to the File structure representing this file.
func (self *Tracker) findFile(name string) *File {
	for _, watcher := range self.watchers {
		if file := watcher.GetFile(name); file != nil {
			return file
		}
	}
	return nil
}

// startSeed attempts to start up a seeding process for a given torrent file.
func (self *Tracker) startSeed(file *File, metadata *Metadata) {
	self.seedStartLock.Lock()

	if file.SeedCommand != nil {
		self.seedStartLock.Unlock()
		return
	}

	tmp, err := ioutil.TempFile("", "distributor.")
	if err != nil {
		logfatal("TempFile failed: %s", err)
	}
	logdebug("Temporary file for %s: %s", file.Name, tmp.Name())

	err = bencode.Marshal(tmp, *metadata)
	if err != nil {
		self.seedStartLock.Unlock()
		logerror("Failed to bencode %s: %s", file.Name, err)
		return
	}

	err = tmp.Sync()
	if err != nil {
		self.seedStartLock.Unlock()
		logerror("Failed to fsync: %s", err)
		return
	}

	file.SeedCommand = exec.Command("/usr/local/bin/ctorrent", "-s", file.FQFN,
		"-e", "4", tmp.Name())
	self.seedStartLock.Unlock()

	// TODO: Read from output pipes, because they could fill up?

	go func() {
		logdebug("Seed starting: %s", file.Name)
		file.SeedCommand.Run()
		logdebug("Seed exited: %s", file.Name)

		// Try to clean up temporary file.
		tmp.Close()
		os.Remove(tmp.Name())

		// Seeds exit after 4 hours. Then they get restarted if someone requests them.
		self.seedStartLock.Lock()
		file.SeedCommand = nil
		self.seedStartLock.Unlock()
	}()
}

// handleServe is the endpoint that is responsible for generating torrent files and giving them
// out to the requestors.
// TODO: how to return 404 etc from here?
func (self *Tracker) handleServe(w http.ResponseWriter, r *http.Request) {
	logdebug("Request: %s", r.URL.RequestURI())
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

	md := Metadata{
		Announce: self.AnnounceURL,
		Info:     *file.MetadataInfo,
	}

	if file.SeedCommand == nil {
		self.startSeed(file, &md)
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

	err := bencode.Marshal(w, md)
	if err != nil {
		logerror("Failed to bencode %s: %s", file.Name, err)
	}
}

// parsePeer extracts a Peer structure from a query string.
func parsePeer(r *http.Request) (string, *Peer, error) {
	values := r.URL.Query()

	var info_hash, peer_id, ip, strport []string
	ok := true

	// I don't know how to make this cleaner in Go. Halp. :-(
	info_hash, ok = values["info_hash"]
	if ok && len(info_hash) == 1 {
		peer_id, ok = values["peer_id"]
	}
	if ok && len(peer_id) == 1 {
		strport, ok = values["port"]
	}
	if !ok {
		return "", nil, errors.New("missing required argument")
	}

	ip, ok = values["ip"]
	if !ok {
		// TODO: This seems fragile.
		addr := strings.Split(r.RemoteAddr, ":")
		if len(addr) != 2 {
			logfatal("Got weird address: %s", r.RemoteAddr)
		}
		ip = []string{addr[0]}
	}

	port, err := strconv.ParseUint(strport[0], 10, 16)
	if err != nil {
		return "", nil, errors.New("port invalid")
	}

	return info_hash[0], &Peer{
		Id:   peer_id[0],
		Ip:   ip[0],
		Port: uint16(port),
	}, nil
}

// handleAnnounce is the endpoint for torrent clients to announce themselves and request
// other peers.
func (self *Tracker) handleAnnounce(w http.ResponseWriter, r *http.Request) {
	info_hash, peer, err := parsePeer(r)
	if err != nil {
		io.WriteString(w, err.Error())
		return
	}
	logdebug("Request from peer at %s:%d.", peer.Ip, peer.Port)

	// Do this now since we're validated our inputs.
	self.peerListLock.Lock()
	defer self.peerListLock.Unlock()

	peers, ok := self.PeerList[info_hash]
	if !ok {
		peers = make(map[string]Peer)
		self.PeerList[info_hash] = peers
	}

	// Add this peer to the set if they don't exist.
	if _, ok := peers[peer.Id]; !ok {
		peers[peer.Id] = *peer
	}

	// We give the user back 50 random peers by just picking a window into our peer list.
	ct := 0
	outPeers := make([]Peer, 0, 50)
	for _, tmpPeer := range peers {
		if tmpPeer.Ip == peer.Ip {
			// This helps avoid giving peers connections to their own machine, which seems
			// to confuse ctorrent. It seems to mostly affect small clusters.
			continue
		}

		outPeers = append(outPeers, tmpPeer)
		logdebug("[%s:%d] %s", peer.Ip, peer.Port, hex.EncodeToString([]byte(tmpPeer.Id)))
		if ct++; ct >= cap(outPeers) {
			break
		}
	}
	logdebug("Giving peer %s:%d a list of %d peers.", peer.Ip, peer.Port, len(outPeers))

	// Build the output dictionary and return it.
	err = bencode.Marshal(w, PeerResponse{Interval: 60, Peers: outPeers})
	if err != nil {
		logerror("Failed to bencode: %s", err)
	}
}

// starTracker spins up a tracker on a given ip:port for the given set of watchers.
func startTracker(ip string, port int, watchers []*Watcher) *Tracker {
	tracker := &Tracker{
		AnnounceURL: fmt.Sprintf("http://%s:%d/announce", ip, port),
		PeerList:    make(map[string]map[string]Peer),
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
