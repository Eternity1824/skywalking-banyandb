// Licensed to Apache Software Foundation (ASF) under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Apache Software Foundation (ASF) licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

// Package fadvis provides utilities for applying posix_fadvise
// to optimize file system performance for large files.
package fadvis

import (
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/apache/skywalking-banyandb/banyand/protector"
	"github.com/apache/skywalking-banyandb/pkg/fadvis"
	"github.com/apache/skywalking-banyandb/pkg/logger"
)

// Default threshold for file size, files larger than this will use fadvis.
var (
	// LargeFileThreshold is the threshold in bytes for considering a file as large.
	LargeFileThreshold atomic.Int64
	// Log is the logger for fadvis related operations.
	Log = logger.GetLogger("fadvis-helper")
	// memoryProtector holds the reference to the Memory protector
	memoryProtector *protector.Memory
	// memoryProtectorMu protects memoryProtector from concurrent access
	memoryProtectorMu sync.RWMutex
	// stopChan is used to signal the background updater to stop
	stopChan chan struct{}
	// started indicates if the background updater has been started
	started bool
	// startMu protects the started flag and init process
	startMu sync.Mutex
)

func init() {
	// Default 64MB.
	LargeFileThreshold.Store(64 * 1024 * 1024)
	// Initialize stop channel
	stopChan = make(chan struct{})
}

// SetMemoryProtector sets the threshold from the Memory protector instance
// and starts a background goroutine to update the threshold every 30 minutes.
func SetMemoryProtector(mp *protector.Memory) {
	startMu.Lock()
	defer startMu.Unlock()

	// Defensive check for nil memory protector
	if mp == nil {
		Log.Warn().Msg("received nil memory protector, using default threshold")
		return
	}

	// Store memory protector for future updates
	memoryProtectorMu.Lock()
	memoryProtector = mp
	memoryProtectorMu.Unlock()

	// Get and set initial threshold
	updateThresholdFromProtector()

	// Start background updater if not already started
	if !started {
		go periodicThresholdUpdater(30 * time.Minute)
		started = true
		Log.Info().Dur("interval", 30*time.Minute).Msg("started periodic threshold updater")
	}
}

// updateThresholdFromProtector gets the threshold from the memory protector and updates the current threshold.
func updateThresholdFromProtector() {
	memoryProtectorMu.RLock()
	mp := memoryProtector
	memoryProtectorMu.RUnlock()

	// Defensive check for nil memory protector
	if mp == nil {
		Log.Warn().Msg("memory protector not set, cannot update threshold")
		return
	}

	// Get threshold from Memory protector with defensive nil check and error handling
	threshold := int64(0)
	defer func() {
		// Recover from any potential panic in GetThreshold
		if r := recover(); r != nil {
			Log.Warn().Interface("recover", r).Msg("recovered from panic in GetThreshold, keeping current threshold")
			return
		}

		// Only set threshold if it's valid
		if threshold > 0 {
			SetThreshold(threshold)
		}
	}()

	// Try to get threshold
	threshold = mp.GetThreshold()
}

// periodicThresholdUpdater runs in the background and updates the threshold periodically.
func periodicThresholdUpdater(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			Log.Debug().Msg("updating threshold from memory protector")
			updateThresholdFromProtector()
		case <-stopChan:
			Log.Info().Msg("stopping periodic threshold updater")
			return
		}
	}
}

// StopUpdater stops the background threshold updater.
func StopUpdater() {
	startMu.Lock()
	defer startMu.Unlock()

	if started {
		close(stopChan)
		started = false
		// Recreate the channel for potential future use
		stopChan = make(chan struct{})
		Log.Info().Msg("stopped periodic threshold updater")
	}
}

// SetThreshold sets the large file threshold.
func SetThreshold(threshold int64) {
	if threshold > 0 {
		LargeFileThreshold.Store(threshold)
		Log.Info().Int64("threshold", threshold).Msg("set large file threshold for fadvis")
	}
}

// GetThreshold returns the current large file threshold.
func GetThreshold() int64 {
	return LargeFileThreshold.Load()
}

// ApplyIfLarge applies fadvis to the file if it's larger than the threshold.
func ApplyIfLarge(filePath string) {
	if fileInfo, err := os.Stat(filePath); err == nil {
		threshold := LargeFileThreshold.Load()
		if fileInfo.Size() > threshold {
			Log.Info().Str("path", filePath).Msg("applying fadvis for large file")
			if err := fadvis.Apply(filePath); err != nil {
				Log.Warn().Err(err).Str("path", filePath).Msg("failed to apply fadvis to file")
			}
		}
	}
}

// MustApplyIfLarge applies fadvis to the file if it's larger than the threshold, panics on error.
func MustApplyIfLarge(filePath string) {
	if fileInfo, err := os.Stat(filePath); err == nil {
		threshold := LargeFileThreshold.Load()
		if fileInfo.Size() > threshold {
			Log.Info().Str("path", filePath).Msg("applying fadvis for large file")
			fadvis.MustApply(filePath)
		}
	}
}
