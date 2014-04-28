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
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Watcher is instantiated for each directory we're serving files for.
type Watcher struct {
	Directory string
	Files     map[string]*File
	FilesLock sync.Mutex
}

// File represents a single file that we are serving. These are read by other parts of the system
// but only written by this module.
type File struct {
	Name         string        // Base filename.
	FullName     string        // Path + filename.
	Size         int64         // File size.
	MetadataInfo *MetadataInfo // Reference to our metadata.
	SeedCommand  *exec.Cmd     // Owned by the Tracker methods.
}

// GetFile returns, given a base filename, either a pointer to a valid file structure or a nil if
// there is no file with that name.
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
		name := <-metaChannel
		//logdebug("Requested metadata generation for: %s", name)

		file := self.GetFile(name)
		if file == nil {
			continue
		}

		info, err := os.Stat(file.FullName)
		if err != nil {
			logerror("Failed to stat %s: %s", file.FullName, err)
			continue
		}

		// If we already have metadata, we also want to check if the size is the same.
		if file.MetadataInfo != nil && file.Size == info.Size() {
			continue
		}

		mdinfo, err := GenerateMetadataInfo(file.FullName)
		if err != nil {
			logfatal("Failed to generate metadata: %s", err)
		}

		info2, err := os.Stat(file.FullName)
		if err != nil {
			logerror("Failed to stat %s: %s", file.FullName, err)
			continue
		}

		if info.Size() != info2.Size() {
			logerror("File changed sizes while generating metadata. Requeuing.")
			metaChannel <- name
			continue
		}

		self.FilesLock.Lock()
		file.Size = info.Size()
		file.MetadataInfo = mdinfo
		self.FilesLock.Unlock()
	}
}

func (self *Watcher) watch() {
	// Set up fsnotify watcher.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logfatal("NewWatcher: %s", err)
	}
	err = watcher.Watch(self.Directory)
	if err != nil {
		logfatal("Watch: %s", err)
	}

	// The watcher is also responsible for (single-threadedly) generating metadata information
	// for files. This is done in such a way as to make it so that files aren't available until
	// the metadata is done.
	metaChannel := make(chan string, 1000)
	go self.metadataGenerator(metaChannel)

	// Now do a quick backfill. We started asking for notifications, which won't tell us about old
	// files. But we are getting notifications now so we should go backfill data for existing
	// files and THEN start listening for updates. Avoids a possible race.
	self.FilesLock.Lock()
	files, err := ioutil.ReadDir(self.Directory)
	if err != nil {
		self.FilesLock.Unlock()
		logfatal("ReadDir: %s", err)
	}
	for _, file := range files {
		name := file.Name()
		if file.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		logdebug("File discovered: %s", name)
		self.Files[name] = &File{
			Name:     name,
			FullName: filepath.Join(self.Directory, name),
		}
		metaChannel <- name
	}
	self.FilesLock.Unlock()

	// This is the main goroutine that actually processes events.
	go func() {
		for {
			select {
			case ev := <-watcher.Event:
				//logdebug("Watcher event: %s", ev)
				func() {
					self.FilesLock.Lock()
					defer self.FilesLock.Unlock()

					name := filepath.Base(ev.Name)
					fqfn := filepath.Join(self.Directory, name)
					info, _ := os.Stat(fqfn)
					_, exists := self.Files[name]

					// We consider modify/attrib updates to also be the same as create, since I've
					// seen at least BSD compress multiple operations into a single call that does
					// not include CREATE. (Happened when testing watching an rsync target.)
					if ev.IsCreate() || ev.IsModify() || ev.IsAttrib() {
						if strings.HasPrefix(name, ".") || info.IsDir() {
							return
						}
						if !exists {
							logdebug("File discovered: %s", name)
						}
						self.Files[name] = &File{
							Name:     name,
							FullName: fqfn,
						}
						metaChannel <- name
					} else if ev.IsRename() || ev.IsDelete() {
						if !exists {
							return
						}
						logdebug("File gone: %s", name)
						delete(self.Files, name)
					}
				}()
			case err := <-watcher.Error:
				logfatal("Watcher error: %s", err)
			}
		}
	}()
}

// startWatcher creates a watcher for a given directory and starts watching it.
func startWatcher(dir string) *Watcher {
	watcher := &Watcher{
		Directory: dir,
		Files:     make(map[string]*File),
	}
	watcher.watch()

	return watcher
}
