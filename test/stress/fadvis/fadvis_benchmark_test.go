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
	"testing"

	"github.com/apache/skywalking-banyandb/banyand/fadvis"
)

// Constants for file sizes and thresholds
const (
	// Large file threshold for fadvis
	DefaultThreshold = 64 * megabyte  // 64MB
	terabyte         = 1 * terabyte   // 1TB (effectively disabled)
	SmallFileSize    = 1 * megabyte   // 1MB
	LargeFileSize    = 256 * megabyte // 256MB
)

// setupTestEnvironment prepares a test environment with the specified fadvis threshold.
// It returns the test directory path and a cleanup function.
func setupTestEnvironment(b *testing.B) (string, func()) {
	// Create a temporary directory for test files
	testDir := b.TempDir()

	// Return cleanup function
	cleanup := func() {
		// Nothing to do as b.TempDir() is automatically cleaned up
	}

	return testDir, cleanup
}

// createLargeFile creates a file of the specified size.
func createLargeFile(path string, size int64) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Truncate to desired size (faster than writing actual data)
	err = file.Truncate(size)
	if err != nil {
		return err
	}

	// Sync to ensure file is persisted to disk
	return file.Sync()
}

// BenchmarkWritePerformance tests write performance with and without fadvis.
func BenchmarkWritePerformance(b *testing.B) {
	// Test cases with different file sizes
	for _, fileSize := range []int64{SmallFileSize, LargeFileSize} {
		// Test with fadvis enabled
		b.Run(fmt.Sprintf("Size_%dMB_FadvisEnabled", fileSize/(1024*1024)), func(b *testing.B) {
			testDir, cleanup := setupTestEnvironment(b)
			defer cleanup()

			// Set the fadvis threshold
			fadvis.SetThreshold(DefaultThreshold)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				filePath := filepath.Join(testDir, fmt.Sprintf("write_test_%d.dat", i))
				if err := createTestFile(b, filePath, fileSize); err != nil {
					b.Fatalf("Failed to create test file: %v", err)
				}

				// Apply fadvis if file is large
				if fileSize > DefaultThreshold {
					fadvis.ApplyIfLarge(filePath)
				}
			}
		})

		// Test with fadvis disabled
		b.Run(fmt.Sprintf("Size_%dMB_FadvisDisabled", fileSize/(1024*1024)), func(b *testing.B) {
			testDir, cleanup := setupTestEnvironment(b)
			defer cleanup()

			// Set the fadvis threshold to a high value to disable it
			fadvis.SetThreshold(terabyte)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				filePath := filepath.Join(testDir, fmt.Sprintf("write_test_%d.dat", i))
				if err := createTestFile(b, filePath, fileSize); err != nil {
					b.Fatalf("Failed to create test file: %v", err)
				}

				// This won't actually apply fadvis due to high threshold
				fadvis.ApplyIfLarge(filePath)
			}
		})
	}
}

// BenchmarkReadPerformance tests read performance with and without fadvis.
func BenchmarkReadPerformance(b *testing.B) {
	// Test cases with different file sizes
	for _, fileSize := range []int64{SmallFileSize, LargeFileSize} {
		// Test with fadvis enabled
		b.Run(fmt.Sprintf("Size_%dMB_FadvisEnabled", fileSize/(1024*1024)), func(b *testing.B) {
			testDir, cleanup := setupTestEnvironment(b)
			defer cleanup()

			// Set the fadvis threshold
			fadvis.SetThreshold(DefaultThreshold)

			// Create a test file
			filePath := filepath.Join(testDir, "read_test.dat")
			if err := createTestFile(b, filePath, fileSize); err != nil {
				b.Fatalf("Failed to create test file: %v", err)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Open and read the file
				data, err := os.ReadFile(filePath)
				if err != nil {
					b.Fatalf("Failed to read file: %v", err)
				}

				// Ensure data is used to prevent compiler optimization
				_ = len(data)

				// Apply fadvis if file is large
				if fileSize > DefaultThreshold {
					fadvis.ApplyIfLarge(filePath)
				}
			}
		})

		// Test with fadvis disabled
		b.Run(fmt.Sprintf("Size_%dMB_FadvisDisabled", fileSize/(1024*1024)), func(b *testing.B) {
			testDir, cleanup := setupTestEnvironment(b)
			defer cleanup()

			// Set the fadvis threshold to a high value to disable it
			fadvis.SetThreshold(terabyte)

			// Create a test file
			filePath := filepath.Join(testDir, "read_test.dat")
			if err := createTestFile(b, filePath, fileSize); err != nil {
				b.Fatalf("Failed to create test file: %v", err)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Open and read the file
				data, err := os.ReadFile(filePath)
				if err != nil {
					b.Fatalf("Failed to read file: %v", err)
				}

				// Ensure data is used to prevent compiler optimization
				_ = len(data)

				// This won't actually apply fadvis due to high threshold
				fadvis.ApplyIfLarge(filePath)
			}
		})
	}
}

// BenchmarkMultipleReads tests the performance impact of multiple reads on the same file.
func BenchmarkMultipleReads(b *testing.B) {
	fileSize := int64(LargeFileSize)
	readCount := 5

	// Test with fadvis enabled
	b.Run("MultipleReads_FadvisEnabled", func(b *testing.B) {
		testDir, cleanup := setupTestEnvironment(b)
		defer cleanup()

		// Set the fadvis threshold
		fadvis.SetThreshold(DefaultThreshold)

		// Create a test file
		filePath := filepath.Join(testDir, "multiple_read_test.dat")
		if err := createTestFile(b, filePath, fileSize); err != nil {
			b.Fatalf("Failed to create test file: %v", err)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for j := 0; j < readCount; j++ {
				// Open and read the file
				data, err := os.ReadFile(filePath)
				if err != nil {
					b.Fatalf("Failed to read file: %v", err)
				}

				// Ensure data is used to prevent compiler optimization
				_ = len(data)

				// Apply fadvis after each read
				fadvis.ApplyIfLarge(filePath)
			}
		}
	})

	// Test with fadvis disabled
	b.Run("MultipleReads_FadvisDisabled", func(b *testing.B) {
		testDir, cleanup := setupTestEnvironment(b)
		defer cleanup()

		// Set the fadvis threshold to a high value to disable it
		fadvis.SetThreshold(terabyte)

		// Create a test file
		filePath := filepath.Join(testDir, "multiple_read_test.dat")
		if err := createTestFile(b, filePath, fileSize); err != nil {
			b.Fatalf("Failed to create test file: %v", err)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for j := 0; j < readCount; j++ {
				// Open and read the file
				data, err := os.ReadFile(filePath)
				if err != nil {
					b.Fatalf("Failed to read file: %v", err)
				}

				// Ensure data is used to prevent compiler optimization
				_ = len(data)

				// This won't actually apply fadvis due to high threshold
				fadvis.ApplyIfLarge(filePath)
			}
		}
	})
}

// BenchmarkMixedWorkload tests the performance of a mixed workload of reads and writes.
func BenchmarkMixedWorkload(b *testing.B) {
	fileSize := int64(LargeFileSize)

	// Test with fadvis enabled
	b.Run("MixedWorkload_FadvisEnabled", func(b *testing.B) {
		testDir, cleanup := setupTestEnvironment(b)
		defer cleanup()

		// Set the fadvis threshold
		fadvis.SetThreshold(DefaultThreshold)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Create a new file
			writeFilePath := filepath.Join(testDir, fmt.Sprintf("mixed_write_%d.dat", i))
			if err := createTestFile(b, writeFilePath, fileSize); err != nil {
				b.Fatalf("Failed to create write file: %v", err)
			}
			fadvis.ApplyIfLarge(writeFilePath)

			// Read the file we just created
			data, err := os.ReadFile(writeFilePath)
			if err != nil {
				b.Fatalf("Failed to read file: %v", err)
			}
			_ = len(data)
			fadvis.ApplyIfLarge(writeFilePath)

			// Create another file
			writeFilePath2 := filepath.Join(testDir, fmt.Sprintf("mixed_write2_%d.dat", i))
			if err := createTestFile(b, writeFilePath2, fileSize); err != nil {
				b.Fatalf("Failed to create second write file: %v", err)
			}
			fadvis.ApplyIfLarge(writeFilePath2)
		}
	})

	// Test with fadvis disabled
	b.Run("MixedWorkload_FadvisDisabled", func(b *testing.B) {
		testDir, cleanup := setupTestEnvironment(b)
		defer cleanup()

		// Set the fadvis threshold to a high value to disable it
		fadvis.SetThreshold(terabyte)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Create a new file
			writeFilePath := filepath.Join(testDir, fmt.Sprintf("mixed_write_%d.dat", i))
			if err := createTestFile(b, writeFilePath, fileSize); err != nil {
				b.Fatalf("Failed to create write file: %v", err)
			}
			fadvis.ApplyIfLarge(writeFilePath)

			// Read the file we just created
			data, err := os.ReadFile(writeFilePath)
			if err != nil {
				b.Fatalf("Failed to read file: %v", err)
			}
			_ = len(data)
			fadvis.ApplyIfLarge(writeFilePath)

			// Create another file
			writeFilePath2 := filepath.Join(testDir, fmt.Sprintf("mixed_write2_%d.dat", i))
			if err := createTestFile(b, writeFilePath2, fileSize); err != nil {
				b.Fatalf("Failed to create second write file: %v", err)
			}
			fadvis.ApplyIfLarge(writeFilePath2)
		}
	})
}
