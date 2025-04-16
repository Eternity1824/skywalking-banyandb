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

// Package fadvis provides utilities for optimizing file system I/O
// by applying POSIX file advice hints.
package fadvis

import (
	"fmt"

	"github.com/apache/skywalking-banyandb/pkg/logger"
)

var (
	// Log is the logger for fadvis operations
	Log = logger.GetLogger("fadvis")
)

// IOTracker tracks file operations for fadvise.
// 它维护文件描述符和操作追踪，通过监控读写操作来应用 fadvise 提示。
// 一个 IOTracker 实例应该与一个文件对应，并在文件操作结束时关闭。
type IOTracker struct {
	fd     int    // 文件描述符
	offset int64  // 当前操作偏移量
	length int64  // 累计操作长度
	path   string // 文件路径（用于日志记录）
}

// NewIOTracker creates a new IOTracker for the given file path.
// 它打开文件并获取文件描述符，但不会关闭文件，而是在 Close 方法中关闭。
func NewIOTracker(path string) (*IOTracker, error) {
	fd, err := openFileDescriptor(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file for tracking: %w", err)
	}

	return &IOTracker{
		fd:     fd,
		path:   path,
		offset: 0,
		length: 0,
	}, nil
}

// Close closes the file descriptor and cleans up resources.
// 在使用完 IOTracker 后必须调用此方法，以避免文件描述符泄漏。
func (t *IOTracker) Close() error {
	if t.fd < 0 {
		return nil // 已经关闭或无效的文件描述符
	}

	err := closeFileDescriptor(t.fd)
	if err == nil {
		t.fd = -1 // 标记为已关闭
	}
	return err
}

// ApplyAfterRead 更新读取操作的追踪信息并应用 fadvise。
// n: 读取的字节数
// offset: 读取操作的偏移量，如果为 -1，则使用当前偏移量
func (t *IOTracker) ApplyAfterRead(n int, offset int64) error {
	if n <= 0 || t.fd < 0 {
		return nil
	}

	t.length += int64(n)
	if offset >= 0 {
		t.offset = offset
	}

	if err := t.applyFadvise(); err != nil {
		Log.Warn().Err(err).
			Str("path", t.path).
			Int64("offset", t.offset).
			Int64("length", t.length).
			Msg("failed to apply fadvise after read")
		return err
	}

	return nil
}

// ApplyAfterWrite 更新写入操作的追踪信息并应用 fadvise。
// n: 写入的字节数
// offset: 写入操作的偏移量，如果为 -1，则使用当前偏移量
func (t *IOTracker) ApplyAfterWrite(n int, offset int64) error {
	if n <= 0 || t.fd < 0 {
		return nil
	}

	t.length += int64(n)
	if offset >= 0 {
		t.offset = offset
	}

	if err := t.applyFadvise(); err != nil {
		Log.Warn().Err(err).
			Str("path", t.path).
			Int64("offset", t.offset).
			Int64("length", t.length).
			Msg("failed to apply fadvise after write")
		return err
	}

	return nil
}

// GetFileDescriptor returns the underlying file descriptor.
func (t *IOTracker) GetFileDescriptor() int {
	return t.fd
}

// GetOffset returns the current offset.
func (t *IOTracker) GetOffset() int64 {
	return t.offset
}

// GetLength returns the cumulative operation length.
func (t *IOTracker) GetLength() int64 {
	return t.length
}

// TrackRead is deprecated, use ApplyAfterRead instead.
func (t *IOTracker) TrackRead(n int, offset int64) error {
	return t.ApplyAfterRead(n, offset)
}

// TrackWrite is deprecated, use ApplyAfterWrite instead.
func (t *IOTracker) TrackWrite(n int, offset int64) error {
	return t.ApplyAfterWrite(n, offset)
}
