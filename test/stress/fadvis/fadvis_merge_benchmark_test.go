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
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/apache/skywalking-banyandb/banyand/fadvis"
	"github.com/stretchr/testify/require"
)

// BenchmarkMergeOperations tests the performance of merge operations with and without fadvis.
func BenchmarkMergeOperations(b *testing.B) {
	if runtime.GOOS != "linux" {
		b.Skip("fadvise is only supported on Linux")
	}

	// Create a temporary directory for the test
	testDir, err := os.MkdirTemp("", "fadvis_merge_benchmark")
	require.NoError(b, err)
	defer os.RemoveAll(testDir)

	// Prepare test files
	parts := createTestParts(b, testDir, 10, LargeFileSize)

	// Run benchmark with fadvise disabled
	b.Run("WithoutFadvise", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			outputFile := filepath.Join(testDir, fmt.Sprintf("merged_%d", i))
			simulateMergeOperation(b, outputFile, parts, false)
		}
	})

	// Run benchmark with fadvise enabled
	b.Run("WithFadvise", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			outputFile := filepath.Join(testDir, fmt.Sprintf("merged_%d", i))
			simulateMergeOperation(b, outputFile, parts, true)
		}
	})
}

// BenchmarkSequentialMergeOperations tests the performance of sequential merge operations.
func BenchmarkSequentialMergeOperations(b *testing.B) {
	if runtime.GOOS != "linux" {
		b.Skip("fadvise is only supported on Linux")
	}

	// Create a temporary directory for the test
	testDir, err := os.MkdirTemp("", "fadvis_merge_benchmark")
	require.NoError(b, err)
	defer os.RemoveAll(testDir)

	// Prepare test files
	parts := createTestParts(b, testDir, 10, LargeFileSize)

	// Run benchmark with fadvise disabled
	b.Run("WithoutFadvise", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			outputFile := filepath.Join(testDir, fmt.Sprintf("merged_%d", i))
			simulateMergeOperation(b, outputFile, parts, false)
		}
	})

	// Run benchmark with fadvise enabled
	b.Run("WithFadvise", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			outputFile := filepath.Join(testDir, fmt.Sprintf("merged_%d", i))
			simulateMergeOperation(b, outputFile, parts, true)
		}
	})
}

// createTestParts prepares test files for merge benchmark.
func createTestParts(b *testing.B, testDir string, numParts int, fileSize int64) []string {
	parts := make([]string, numParts)
	for i := 0; i < numParts; i++ {
		partPath := filepath.Join(testDir, fmt.Sprintf("part_%d", i))
		err := createTestFile(b, partPath, fileSize)
		require.NoError(b, err)
		parts[i] = partPath
	}
	return parts
}

// simulateMergeOperation simulates a merge operation by reading parts and writing to an output file.
func simulateMergeOperation(b *testing.B, outputFile string, parts []string, useFadvise bool) {
	out, err := os.Create(outputFile)
	require.NoError(b, err)
	defer out.Close()

	// Read from parts and write to output file
	buffer := make([]byte, 8192)
	for _, part := range parts {
		in, err := os.Open(part)
		require.NoError(b, err)

		for {
			n, err := in.Read(buffer)
			if n == 0 || err != nil {
				break
			}

			_, err = out.Write(buffer[:n])
			require.NoError(b, err)
		}

		in.Close()
	}

	// Sync to ensure data is written
	err = out.Sync()
	require.NoError(b, err)

	// Apply fadvise to merged files if enabled
	if useFadvise {
		// Apply fadvise to all large files in the merged part
		fadvis.ApplyIfLarge(outputFile)
	}
}

// createTestFile creates a test file of the specified size.
func createTestFile(b *testing.B, path string, size int64) {
	file, err := os.Create(path)
	if err != nil {
		b.Fatalf("Failed to create file %s: %v", path, err)
	}
	defer file.Close()

	// Allocate the file to the specified size
	err = file.Truncate(size)
	if err != nil {
		b.Fatalf("Failed to truncate file %s to size %d: %v", path, size, err)
	}

	// Sync the file to ensure it's written to disk
	err = file.Sync()
	if err != nil {
		b.Fatalf("Failed to sync file %s: %v", path, err)
	}
}
