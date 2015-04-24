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

var VERBOSITY uint32 = 0

func SetLoggingVerbosity(level uint32) {
	VERBOSITY = level
}

func LogInfo(fmt string, args ...interface{}) {
	if VERBOSITY >= 1 {
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
	if VERBOSITY >= 2 {
		log.Printf("[DEBUG] "+fmt, args...)
	}
}
