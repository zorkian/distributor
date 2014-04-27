/*
 * logging.go
 *
 * Copyright (c) 2014 by authors and contributors. Please see the included LICENSE file for
 * licensing information.
 *
 */

package main

import (
	"log"
)

func loginfo(fmt string, args ...interface{}) {
	if verbose >= 1 {
		log.Printf("[INFO] "+fmt, args...)
	}
}

func logwarning(fmt string, args ...interface{}) {
	log.Printf("[WARNING] "+fmt, args...)
}

func logerror(fmt string, args ...interface{}) {
	log.Printf("[ERROR] "+fmt, args...)
}

func logfatal(fmt string, args ...interface{}) {
	log.Fatalf("[FATAL] "+fmt, args...)
}

func logdebug(fmt string, args ...interface{}) {
	if verbose >= 2 {
		log.Printf("[DEBUG] "+fmt, args...)
	}
}
