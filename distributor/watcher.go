/*
 * watcher.go
 *
 * Copyright (c) 2014 by authors and contributors. Please see the included LICENSE file for
 * licensing information.
 *
 */

package main

import (
	"github.com/howeyc/fsnotify"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Watcher is instantiated for each directory we're serving files for.
type Watcher struct {
	Watcher   *fsnotify.Watcher
	Directory string
	Files     map[string]*File // FQFN as key.
	FilesLock sync.Mutex
}

// File represents a single file that we are serving. These are read by other parts of the system
// but only written by this module.
type File struct {
	Name         string        // Base filename.
	FQFN         string        // Path + filename.
	Size         int64         // File size.
	MetadataInfo *MetadataInfo // Reference to our metadata.
	SeedCommand  *exec.Cmd     // Owned by the Tracker methods.
}

// GetFile returns, given a full path filename, either a pointer to a valid file structure or a
// nil if there is no file with that name.
func (self *Watcher) GetFile(name string) *File {
	self.FilesLock.Lock()
	defer self.FilesLock.Unlock()

	return self.Files[name]
}

func (self *Watcher) metadataGenerator(metaChannel chan string) {
	// Some assumptions: We are the only writer to ever touch the Metadata record in any
	// File object globally. We take a lock to get the file and before we do any manipulation
	// of the structures, but otherwise we do NOT lock during the metadata generation stage since
	// it can take a while.
	for {
		localfn := <-metaChannel

		file := self.GetFile(localfn)
		if file == nil {
			continue
		}

		info, err := os.Stat(file.FQFN)
		if err != nil {
			logerror("Failed to stat %s: %s", file.FQFN, err)
			continue
		}

		// If we already have metadata, we also want to check if the size is the same.
		if file.MetadataInfo != nil && file.Size == info.Size() {
			continue
		}

		mdinfo, err := GenerateMetadataInfo(file.FQFN)
		if err != nil {
			logfatal("Failed to generate metadata: %s", err)
		}

		info2, err := os.Stat(file.FQFN)
		if err != nil {
			logerror("Failed to stat %s: %s", file.FQFN, err)
			continue
		}

		if info.Size() != info2.Size() {
			logerror("File changed sizes while generating metadata. Requeuing.")
			metaChannel <- localfn
			continue
		}

		file.Size = info.Size()
		file.MetadataInfo = mdinfo
	}
}

func (self *Watcher) updateChannelHandler(updates chan string) {
	// The watcher is also responsible for (single-threadedly) generating metadata information
	// for files. This is done in such a way as to make it so that files aren't available until
	// the metadata is done.
	metaChannel := make(chan string, 10000)
	go self.metadataGenerator(metaChannel)

	for {
		fqfn := <-updates

		if !strings.HasPrefix(fqfn, self.Directory) {
			logerror("File %s not in watched dir %s!", fqfn, self.Directory)
			continue
		}
		localfn := fqfn[len(self.Directory)+1:]
		requestMetadata := false

		func() {
			self.FilesLock.Lock()
			defer self.FilesLock.Unlock()

			info, _ := os.Stat(fqfn)
			_, exists := self.Files[localfn]
			name := filepath.Base(fqfn)

			if exists && info == nil {
				// Deleted files.
				logdebug("File removed: %s", fqfn)
				delete(self.Files, fqfn)

			} else if !exists && info != nil {
				// New file found, watch it or add it to our list.
				if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".mdcache") {
					return
				}

				// Directories get walked, files just get added.
				if info.IsDir() {
					go self.walkAndWatch(fqfn, updates)
				} else {
					logdebug("File discovered: %s", localfn)
					self.Files[localfn] = &File{
						Name: name,
						FQFN: fqfn,
					}
					requestMetadata = true
				}
			}
		}()

		// This has to happen late like this instead of above since otherwise we might end up
		// with deadlock with the metadata generator.
		if requestMetadata {
			metaChannel <- localfn
		}
	}
}

func (self *Watcher) walkAndWatch(dir string, updates chan string) {
	logdebug("Walking directory: %s", dir)
	filepath.Walk(dir, func(fqfn string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			loginfo("Watching directory: %s", fqfn)
			if err := self.Watcher.Watch(fqfn); err != nil {
				logfatal("Watch: %s", err)
			}
		} else {
			updates <- fqfn
		}
		return nil
	})
}

func (self *Watcher) watch() {
	// Set up our change channel. This is sent notifications whenever a file event has happened,
	// and it's responsible for updating local status.
	updateChannel := make(chan string, 1000)
	go self.updateChannelHandler(updateChannel)

	// Walks a directory and watches everything in it.
	self.walkAndWatch(self.Directory, updateChannel)

	// This is the main goroutine that actually processes events.
	for {
		select {
		case ev := <-self.Watcher.Event:
			// Regardless of what the event is, just let the update channel know something has
			// updated. It can infer what it needs to do based on the present state.
			updateChannel <- ev.Name
		case err := <-self.Watcher.Error:
			logerror("Watcher error: %s", err)
		}
	}
	logfatal("Watcher exited unexpectedly.")
}

// startWatcher creates a watcher for a given directory and starts watching it.
func startWatcher(dir string) *Watcher {
	// Set up fsnotify watcher.
	fswatcher, err := fsnotify.NewWatcher()
	if err != nil {
		logfatal("NewWatcher: %s", err)
	}

	watcher := &Watcher{
		Watcher:   fswatcher,
		Directory: dir,
		Files:     make(map[string]*File),
	}
	go watcher.watch()

	return watcher
}
