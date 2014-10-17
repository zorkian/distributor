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
	"errors"
	"fmt"
	bencode "github.com/jackpal/bencode-go"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
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
	// We keep a separate set of peers for each info_hash. We don't actually verify that these
	// hashes are valid; so there's a pretty easy DoS here. This system is designed to be used
	// in a production environment with good actors. TODO: harden.
	// TODO: We need a way of droppign peers that have not reported in a while.
	PeerSeen     map[string]map[string]time.Time
	PeerList     map[string]map[string]Peer
	peerListLock sync.Mutex

	// Lock used by all methods that affect the seed process.
	seedStartLock sync.Mutex

	// The key in the watchers map is how these watchers can be queried for the latest data
	// see handleServeLastUpdated()
	//
	// Careful: there is no locking here. It's assumed that the only time this is
	// written is from the very initial setup of the app and never during runtime. If that
	// changes we'll need locking. (This may actually be technically a little racy right
	// now if there's a ton of requests during power-on, since we start listening
	// before the watchers are created.)
	watchers map[string]*Watcher // List of watchers who might have files.
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

// findLastUpdatedFile goes through all the watchers and returns the file with the latest
// modification time or nil if no such file could be found; only considers files that have
// non-nil metadata, as there are cases where we don't generate metadata for files
// that exist (e.g. 0-length)
func (self *Tracker) findLastUpdatedFile(watchers []*Watcher) *File {
	var last_updated *File = nil
	for _, watcher := range watchers {
		for _, file := range watcher.GetFiles() {
			if file.MetadataInfo == nil {
				continue
			}

			if last_updated == nil || file.ModTime.After(last_updated.ModTime) {
				last_updated = file
			}
		}
	}
	return last_updated
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

	file.SeedCommand = exec.Command(CTORRENT, "-s", file.FQFN, "-e", "4", "-p", "8999", tmp.Name())
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
	self.serveFile(w, r, file)
}

// handleServeLatest is the endpoint that is responsible for serving the latest file that was updated
func (self *Tracker) handleServeLastUpdated(w http.ResponseWriter, r *http.Request) {
	logdebug("Request: %s", r.URL.RequestURI())

	var query_watchers []*Watcher

	pieces := strings.SplitN(r.URL.RequestURI(), "?", 2)
	if len(pieces) > 2 {
		io.WriteString(w, "invalid request")
		return
	} else if len(pieces) == 2 {
		// query the specified watcher
		watcher := self.watchers[pieces[1]]
		if watcher == nil {
			io.WriteString(w, "invalid watcher name")
			return
		}
		query_watchers = append(query_watchers, watcher)
	} else {
		// query all watchers
		for _, watcher := range self.watchers {
			query_watchers = append(query_watchers, watcher)
		}
	}

	file := self.findLastUpdatedFile(query_watchers)
	self.serveFile(w, r, file)
}

func (self *Tracker) serveFile(w http.ResponseWriter, r *http.Request, file *File) {
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
		// Using Host like this is probably safe, but is potentially a hack.
		Announce: fmt.Sprintf("http://%s/announce", r.Host),
		Info:     *file.MetadataInfo,
	}

	if file.SeedCommand == nil {
		self.startSeed(file, &md)
	}

	err := bencode.Marshal(w, md)
	if err != nil {
		logerror("Failed to bencode %s: %s", file.Name, err)
	}
}

// parsePeer extracts a Peer structure from a query string.
func parsePeer(r *http.Request, values url.Values) (*Peer, error) {
	var peer_id, ip, strport []string
	ok := true

	// I don't know how to make this cleaner in Go. Halp. :-(
	peer_id, ok = values["peer_id"]
	if ok && len(peer_id) == 1 {
		strport, ok = values["port"]
	}
	if !ok {
		return nil, errors.New("missing required argument")
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
		return nil, errors.New("port invalid")
	}

	return &Peer{
		Id:   peer_id[0],
		Ip:   ip[0],
		Port: uint16(port),
	}, nil
}

// handleAnnounce is the endpoint for torrent clients to announce themselves and request
// other peers.
func (self *Tracker) handleAnnounce(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()

	peer, err := parsePeer(r, values)
	if err != nil {
		io.WriteString(w, err.Error())
		return
	}
	logdebug("Request from peer at %s:%d.", peer.Ip, peer.Port)

	// Get other arguments and validate them.
	var info_hash string
	if info_hash_list, ok := values["info_hash"]; ok && len(info_hash_list) == 1 {
		info_hash = info_hash_list[0]
	}

	var event string
	if event_list, ok := values["event"]; ok && len(event_list) == 1 {
		event = event_list[0]
	}

	var numwant uint64
	if numwant_list, ok := values["numwant"]; ok && len(numwant_list) == 1 {
		numwant, err = strconv.ParseUint(numwant_list[0], 10, 8)
		if err != nil || numwant > 100 {
			numwant = 100
		}
	} else {
		numwant = 50
	}

	// Lock this now since we're validated our inputs.
	self.peerListLock.Lock()
	defer self.peerListLock.Unlock()

	peers, ok := self.PeerList[info_hash]
	if !ok {
		peers = make(map[string]Peer)
		self.PeerList[info_hash] = peers
	}

	peerseen, ok := self.PeerSeen[info_hash]
	if !ok {
		peerseen = make(map[string]time.Time)
		self.PeerSeen[info_hash] = peerseen
	}

	var peerage time.Duration
	peerlastseen, pok := peerseen[peer.Id]
	if pok {
		peerage = time.Since(peerlastseen)
	}

	// Add this peer to the set if they don't exist, plus possibly purge other peers on this IP.
	if _, ok := peers[peer.Id]; !ok {
		// Remove any other peers on this IP address. This is kind of a hack since we don't have
		// "last reported time" at the moment. If a new peer starts up on a host, then we remove
		// the other one.
		toRemove := make([]string, 0, 10)
		for id, tmpPeer := range peers {
			if tmpPeer.Ip == peer.Ip {
				toRemove = append(toRemove, id)
			} else if pok && peerage > 300*time.Second {
				// This gives us a 1% chance that every time we iterate over this list, we remove
				// the peer. This is a gross hack to provide removal of dead peers eventually, this
				// should really be time based.
				toRemove = append(toRemove, id)
			}
		}
		for _, id := range toRemove {
			delete(peers, id)
			delete(peerseen, id)
		}

		// Finally insert this new peer.
		peers[peer.Id] = *peer
	}

	// Always update the timestamp so we know when people report.
	peerseen[peer.Id] = time.Now()

	// If they're stopping, then remove this peer from the valid list.
	if event == "stopped" {
		loginfo("Peer %s:%d is leaving the swarm.", peer.Ip, peer.Port)
		delete(peers, peer.Id)
	}

	// We give the user back N random peers by just picking a window into our peer list.
	ct := 0
	outPeers := make([]Peer, 0, numwant)
	for _, tmpPeer := range peers {
		if ct++; ct > cap(outPeers) {
			break
		}

		if tmpPeer.Ip == peer.Ip {
			// This helps avoid giving peers connections to their own machine, which seems
			// to confuse ctorrent. It seems to mostly affect small clusters.
			continue
		}
		outPeers = append(outPeers, tmpPeer)
		logdebug("[%s:%d] peer %s:%d", peer.Ip, peer.Port, tmpPeer.Ip, tmpPeer.Port)
	}
	loginfo("Giving peer %s:%d a list of %d peers (out of %d).",
		peer.Ip, peer.Port, len(outPeers), len(peers))

	// Build the output dictionary and return it.
	err = bencode.Marshal(w, PeerResponse{Interval: rand.Intn(120) + 300, Peers: outPeers})
	if err != nil {
		logerror("Failed to bencode: %s", err)
	}
}

// starTracker spins up a tracker on a given ip:port for the given set of watchers.
func startTracker(ip string, port int, watchers map[string]*Watcher) *Tracker {
	tracker := &Tracker{
		PeerList: make(map[string]map[string]Peer),
		PeerSeen: make(map[string]map[string]time.Time),
		watchers: watchers,
	}

	http.HandleFunc("/serve", tracker.handleServe)
	http.HandleFunc("/serve_last_updated", tracker.handleServeLastUpdated)
	http.HandleFunc("/announce", tracker.handleAnnounce)

	go func() {
		err := http.ListenAndServe(fmt.Sprintf("%s:%d", ip, port), nil)
		logfatal("HTTP server exited: %s", err)
	}()

	return tracker
}
