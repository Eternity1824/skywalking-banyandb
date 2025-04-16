// Licensed to Apache Software Foundation (ASF) under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Apache Software Foundation (ASF) licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package fadvis

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/apache/skywalking-banyandb/banyand/fadvis"
	"github.com/stretchr/testify/require"
)

// Constants for file sizes and thresholds
const (
	kilobyte = 1024
	megabyte = 1024 * 1024
	gigabyte = 1024 * 1024 * 1024
	terabyte = 1024 * 1024 * 1024 * 1024

	// Default threshold for large files (100MB)
	DefaultThreshold = 100 * megabyte
	// Small file size (10MB)
	SmallFileSize = 10 * megabyte
	// Large file size (200MB)
	LargeFileSize = 200 * megabyte
	// Default concurrency level
	DefaultConcurrency = 4
)

func init() {
	// Seed the random number generator
	rand.Seed(time.Now().UnixNano())
}

// createTestFile creates a test file of the specified size.
// It automatically applies fadvise if the file size exceeds the threshold.
func createTestFile(t testing.TB, filePath string, size int64) error {
	// Create parent directories if they don't exist
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}

	// Create the file
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Truncate to the desired size
	if err := file.Truncate(size); err != nil {
		return err
	}

	// Apply fadvise if file size exceeds threshold
	if size > fadvis.GetThreshold() {
		fadvis.ApplyIfLarge(filePath)
	}

	// Sync to ensure the file is written to disk
	return file.Sync()
}

// readFileWithFadvise reads a file with automatic fadvise application.
// It applies fadvise if the file size exceeds the threshold.
func readFileWithFadvise(t testing.TB, filePath string) ([]byte, error) {
	// Check file size
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	// Apply fadvise if file size exceeds threshold
	if info.Size() > fadvis.GetThreshold() {
		fadvis.ApplyIfLarge(filePath)
	}

	// Read the file
	return os.ReadFile(filePath)
}

// appendToFile appends data to a file, creating it if it doesn't exist.
// It automatically applies fadvise if the file size exceeds the threshold.
func appendToFile(filePath string, data []byte) error {
	// Check if file exists and get its size
	info, err := os.Stat(filePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file for append: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	// Apply fadvise if total size exceeds threshold
	if info != nil && info.Size()+int64(len(data)) > fadvis.GetThreshold() {
		fadvis.ApplyIfLarge(filePath)
	}

	return f.Sync()
}

// setupTestEnvironment creates a test directory and returns a cleanup function.
func setupTestEnvironment(t testing.TB) (string, func()) {
	tempDir := t.TempDir()
	return tempDir, func() {}
}

// createTestParts creates a set of test parts for merge benchmark.
func createTestParts(t testing.TB, testDir string, numParts int, partSize int64) []string {
	parts := make([]string, numParts)
	for i := 0; i < numParts; i++ {
		partPath := filepath.Join(testDir, fmt.Sprintf("part_%d", i))
		err := createTestFile(t, partPath, partSize)
		require.NoError(t, err)
		parts[i] = partPath
	}
	return parts
}

// simulateMergeOperation simulates a merge operation by reading parts and writing to an output file.
func simulateMergeOperation(t testing.TB, outputFile string, parts []string) {
	out, err := os.Create(outputFile)
	require.NoError(t, err)
	defer out.Close()

	// Read from parts and write to output file
	buffer := make([]byte, 8192)
	for _, part := range parts {
		in, err := os.Open(part)
		require.NoError(t, err)

		for {
			n, err := in.Read(buffer)
			if n == 0 || err != nil {
				break
			}

			_, err = out.Write(buffer[:n])
			require.NoError(t, err)
		}

		in.Close()
	}

	// Sync to ensure data is written
	err = out.Sync()
	require.NoError(t, err)
}
