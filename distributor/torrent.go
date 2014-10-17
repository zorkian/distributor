/*
 * torrent.go
 *
 * Implementation for generating torrent metadata.
 *
 * Copyright (c) 2014 by authors and contributors. Please see the included LICENSE file for
 * licensing information.
 *
 */

package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
)

type Metadata struct {
	Announce string       `announce` // URL of our tracker.
	Info     MetadataInfo `info`
}

type MetadataInfo struct {
	Name        string `name`         // Filename.
	PieceLength int    `piece length` // Size of pieces.
	Pieces      string `pieces`       // The actual pieces data.
	Length      int64  `length`
}

// GenerateMetadata takes a file and generates the metadata required to serve that file.
func GenerateMetadataInfo(fqfn string) (*MetadataInfo, error) {
	info, err := os.Stat(fqfn)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(fqfn)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Sometimes we get 0 length files to begin with. In those cases, do nothing. It's also
	// not considered an error.
	if info.Size() == 0 {
		return nil, nil
	}

	// TODO: We might want to do something intelligent to vary this, but 256kb is now the
	// pseudo-standard for BT pieces and is reasonable (metadata file is ~1MB for an 8GB
	// file being served).
	pieceLength := int64(256 * 1024)
	pieceCount := int(math.Ceil(float64(info.Size()) / float64(pieceLength)))
	pieces := make([][]byte, 0, pieceCount)

	// See if we've already cached this file's hash information.
	use_cache := false
	cache_bytes := []byte{}
	cache_fqfn := fqfn + ".mdcache"
	cache_info, err := os.Stat(cache_fqfn)
	if err == nil && cache_info != nil {
		if info.ModTime().After(cache_info.ModTime()) {
			logdebug("Cache invalid: %s updated more recently than %s", fqfn, cache_fqfn)
		} else {
			cache_bytes, err = ioutil.ReadFile(cache_fqfn)
			if err == nil {
				logdebug("Loaded %d cached bytes from %s.", len(cache_bytes), cache_fqfn)
				if len(cache_bytes) != pieceCount*20 {
					logerror("Cache invalid: length does not match expected size!")
				} else {
					use_cache = true
				}
			}
		}
	}

	// If we had a cache, break it into the pieces.
	if use_cache {
		for i := 0; i < pieceCount; i++ {
			idx := i * 20
			pieces = append(pieces, cache_bytes[idx:idx+20])
		}
	} else {
		// Calculate the SHA1 string by chunking the file.
		buf := make([]byte, pieceLength)
		bytesRead, fileSize := int64(0), info.Size()
		for {
			bytesToRead := fileSize - bytesRead

			// There is a case where bytesToRead becomes negative, because the file has grown
			// while we were reading it. In this case, bail with nothing. The caller will notice
			// that the file has changed size and check it again.
			if bytesToRead < 0 {
				logdebug("Metadata generator found growing file: %s", fqfn)
				return nil, nil
			}

			if bytesToRead > pieceLength {
				bytesToRead = pieceLength
			}
			if bytesToRead == 0 {
				break
			}

			n, err := io.ReadAtLeast(file, buf, int(bytesToRead))
			if n == 0 && err == io.EOF {
				break
			} else if err != nil {
				logfatal("Failed to read: %s", err)
			}

			hash := sha1.New()
			nw, err := hash.Write(buf[:n])
			if err != nil {
				logfatal("Failed to hash: %s", err)
			} else if nw != n {
				logfatal("Failed to write to hash; %d != %d", n, nw)
			}
			pieces = append(pieces, hash.Sum(nil))
			bytesRead += int64(n)
		}

		// Final sanity check: bytesRead should exactly equal the file size.
		if int64(bytesRead) != info.Size() {
			logfatal("Read %d, size %d... mismatch!", bytesRead, info.Size())
		}

		// Write out cache file.
		if err := ioutil.WriteFile(cache_fqfn, bytes.Join(pieces, []byte{}), 0644); err != nil {
			logerror("Failed to write cache file: %s", err)
		}
	}

	logdebug("Generated (or cached) metadata for %s:", fqfn)
	logdebug(" * Pieces:     %d * %d bytes", pieceCount, pieceLength)
	logdebug(" * First hash: %s", hex.EncodeToString(pieces[0]))

	// Build and return metadata structure, after caching it.
	return &MetadataInfo{
		Name:        filepath.Base(fqfn),
		PieceLength: int(pieceLength),
		Pieces:      string(bytes.Join(pieces, []byte{})),
		Length:      info.Size(),
	}, nil
}
