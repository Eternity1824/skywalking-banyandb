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
//go:build !linux
// +build !linux

// Package fadvis provides file system hint utilities.
// This file contains non-Linux stub implementations that no-op
// to ensure cross-platform compatibility.
package fadvis

// Apply is a no-op on non-Linux platforms.
// The POSIX_FADV_DONTNEED functionality is specific to Linux.
func Apply(_ string) error {
	// No-op on non-Linux platforms
	return nil
}

// MustApply is a no-op on non-Linux platforms.
func MustApply(_ string) {
	// No-op
}

// ApplySequential is a no-op on non-Linux platforms.
func ApplySequential(_ string) error {
	return nil
}

// MustApplySequential is a no-op on non-Linux platforms.
func MustApplySequential(_ string) {
	// No-op
}
