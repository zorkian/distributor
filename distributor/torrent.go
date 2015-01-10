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
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
)

// 256kb is now the pseudo-standard for BT pieces and is reasonable (metadata file is ~1MB
// for an 8GB file being served).
const PIECE_LENGTH = int64(256 * 1024)

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

// makeHashes takes a file, chunks it into pieces, and calculates SHA1 hashes for each of the
// chunks.
func makeHashes(data io.Reader, dataSize int64) ([][]byte, int64, error) {
	hashCount := int(math.Ceil(float64(dataSize) / float64(PIECE_LENGTH)))
	hashes := make([][]byte, 0, hashCount)
	buf := make([]byte, PIECE_LENGTH)
	bytesRead := int64(0)
	for {
		bytesToRead := dataSize - bytesRead

		// There is a case where bytesToRead becomes negative, because the file has grown
		// while we were reading it. In this case, bail with nothing. The caller will notice
		// that the file has changed size and check it again.
		if bytesToRead < 0 {
			return nil, 0, nil
		}

		if bytesToRead > PIECE_LENGTH {
			bytesToRead = PIECE_LENGTH
		}
		if bytesToRead == 0 {
			break
		}

		n, err := io.ReadAtLeast(data, buf, int(bytesToRead))
		if n == 0 && err == io.EOF {
			break
		} else if err != nil {
			return nil, 0, errors.New(fmt.Sprintf("Failed to read: %s", err))
		}

		hash := sha1.New()
		nw, err := hash.Write(buf[:n])
		if err != nil {
			return nil, 0, errors.New(fmt.Sprintf("Failed to hash chunk: %s", err))
		} else if nw != n {
			return nil, 0, errors.New(fmt.Sprintf("Failed to write to hash; %d != %d", n, nw))
		}
		hashes = append(hashes, hash.Sum(nil))
		bytesRead += int64(n)
	}

	return hashes, bytesRead, nil
}

// GenerateMetadata takes a file and generates the metadata required to serve that file.
func GenerateMetadataInfo(fqfn string) (*MetadataInfo, error) {
	info, err := os.Stat(fqfn)
	if err != nil {
		return nil, err
	}

	// Sometimes we get 0 length files to begin with. In those cases, do nothing. It's also
	// not considered an error.
	if info.Size() == 0 {
		return nil, nil
	}

	file, err := os.Open(fqfn)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	hashCount := int(math.Ceil(float64(info.Size()) / float64(PIECE_LENGTH)))
	var hashes [][]byte

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
				if len(cache_bytes) != hashCount*20 {
					logerror("Cache invalid: length does not match expected size!")
				} else {
					use_cache = true
				}
			}
		}
	}

	// If we had a cache, break it into the hashes.
	if use_cache {
		hashes := make([][]byte, 0, hashCount)
		for i := 0; i < hashCount; i++ {
			idx := i * 20
			hashes = append(hashes, cache_bytes[idx:idx+20])
		}
	} else {
		var bytesRead int64
		var err error
		hashes, bytesRead, err = makeHashes(file, info.Size())
		if err != nil {
			logfatal("Failed to make hashes for file: %s", err)
		}

		// Final sanity check: bytesRead should exactly equal the file size.
		if int64(bytesRead) != info.Size() {
			logfatal("Read %d, size %d... mismatch!", bytesRead, info.Size())
		}

		// Write out cache file.
		if err := ioutil.WriteFile(cache_fqfn, bytes.Join(hashes, []byte{}), 0644); err != nil {
			logerror("Failed to write cache file: %s", err)
		}
	}

	logdebug("Generated (or cached) metadata for %s:", fqfn)
	logdebug(" * Pieces:     %d * %d bytes", hashCount, PIECE_LENGTH)
	logdebug(" * First hash: %s", hex.EncodeToString(hashes[0]))

	// Build and return metadata structure, after caching it.
	return &MetadataInfo{
		Name:        filepath.Base(fqfn),
		PieceLength: int(PIECE_LENGTH),
		Pieces:      string(bytes.Join(hashes, []byte{})),
		Length:      info.Size(),
	}, nil
}
