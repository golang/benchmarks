// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

const idleMaxLoad = 0.2

// loadAvg returns the 1-minute load average.
func loadAvg() (float64, error) {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, fmt.Errorf("error reading /proc/loadavg: %w", err)
	}

	s := strings.TrimSpace(string(b))
	log.Printf("Load average: %s", s)

	parts := strings.Split(s, " ")

	avg, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, fmt.Errorf("malformed load average %q: %v", parts[0], err)
	}
	return avg, nil
}

func waitForIdle() error {
	avg, err := loadAvg()
	if err != nil {
		return fmt.Errorf("error reading load average: %w", err)
	}
	if avg < idleMaxLoad {
		return nil
	}

	log.Printf("Waiting for load average to drop below %.2f...", idleMaxLoad)

	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()

	for _ = range tick.C {
		avg, err := loadAvg()
		if err != nil {
			return fmt.Errorf("error reading load average: %w", err)
		}
		if avg < idleMaxLoad {
			break
		}

		log.Printf("Waiting for load average to drop below %.2f...", idleMaxLoad)
	}

	return nil
}
