/*
 * logging.go
 *
 * Copyright (c) 2014 by authors and contributors. Please see the included LICENSE file for
 * licensing information.
 *
 */

package torrent

import (
	"log"
)

type Verbosity uint32

const (
	VerbNormal  = Verbosity(0)
	VerbVerbose = Verbosity(1)
	VerbDebug   = Verbosity(2)
)

var VERBOSITY Verbosity = VerbNormal

func SetLoggingVerbosity(level Verbosity) {
	VERBOSITY = level
}

func LogInfo(fmt string, args ...interface{}) {
	if VERBOSITY >= VerbVerbose {
		log.Printf("[INFO] "+fmt, args...)
	}
}

func LogWarning(fmt string, args ...interface{}) {
	log.Printf("[WARNING] "+fmt, args...)
}

func LogError(fmt string, args ...interface{}) {
	log.Printf("[ERROR] "+fmt, args...)
}

func LogFatal(fmt string, args ...interface{}) {
	log.Fatalf("[FATAL] "+fmt, args...)
}

func LogDebug(fmt string, args ...interface{}) {
	if VERBOSITY >= VerbDebug {
		log.Printf("[DEBUG] "+fmt, args...)
	}
}
