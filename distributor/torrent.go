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
	Pieces      []byte `pieces`       // The actual pieces data.
	Length      int64  `length`
}

// GenerateMetadata takes a file and a tracker and generates the appropriate structure for
// that file to be served by that tracker.
func GenerateMetadata(fqfn string, tracker *Tracker) (*Metadata, error) {
	info, err := os.Stat(fqfn)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(fqfn)
	if err != nil {
		return nil, err
	}

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

	// Calculate the SHA1 string by chunking the file.
	buf := make([]byte, pieceLength)
	bytesRead, fileSize := int64(0), info.Size()
	for {
		bytesToRead := fileSize - bytesRead
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

	logdebug("Generated metadata for %s:", fqfn)
	logdebug(" * Pieces:     %d * %d bytes", pieceCount, pieceLength)
	logdebug(" * First hash: %s", hex.EncodeToString(pieces[0]))

	// Build and return metadata structure
	return &Metadata{
		Announce: tracker.AnnounceURL,
		Info: MetadataInfo{
			Name:        filepath.Base(fqfn),
			PieceLength: int(pieceLength),
			Pieces:      bytes.Join(pieces, []byte{}),
			Length:      bytesRead,
		},
	}, nil
}
