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
//go:build linux
// +build linux

package fadvis

import (
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

// 文件到文件描述符的缓存，避免重复打开文件
var (
	fdCache     = make(map[string]*fdEntry)
	fdCacheLock sync.RWMutex
)

type fdEntry struct {
	fd     int
	file   *os.File
	refCnt int
}

// Apply applies POSIX_FADV_DONTNEED to the specified file path.
// This tells the kernel that the file data is not expected to be accessed
// in the near future, allowing it to be removed from the page cache.
func Apply(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// Ensure data is synced to disk before advising
	if err := f.Sync(); err != nil {
		return err
	}

	return unix.Fadvise(int(f.Fd()), 0, 0, unix.FADV_DONTNEED)
}

// MustApply applies POSIX_FADV_DONTNEED to the specified file path and panics on error
func MustApply(path string) {
	if err := Apply(path); err != nil {
		panic(err)
	}
}

// ApplySequential applies POSIX_FADV_SEQUENTIAL to the specified file path.
// This hint tells the kernel that the application is going to read from the file sequentially.
func ApplySequential(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	return unix.Fadvise(int(f.Fd()), 0, 0, unix.FADV_SEQUENTIAL)
}

// MustApplySequential applies POSIX_FADV_SEQUENTIAL to the specified file path and panics on error
func MustApplySequential(path string) {
	if err := ApplySequential(path); err != nil {
		panic(err)
	}
}

// openFileDescriptor opens a file and returns its file descriptor from cache or by opening the file.
func openFileDescriptor(path string) (int, error) {
	fdCacheLock.RLock()
	entry, ok := fdCache[path]
	fdCacheLock.RUnlock()

	if ok {
		fdCacheLock.Lock()
		entry.refCnt++
		fdCacheLock.Unlock()
		return entry.fd, nil
	}

	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return -1, err
	}

	fd := int(f.Fd())

	if err := f.Sync(); err != nil {
		f.Close()
		return -1, err
	}

	fdCacheLock.Lock()
	fdCache[path] = &fdEntry{fd: fd, file: f, refCnt: 1}
	fdCacheLock.Unlock()

	return fd, nil
}

// closeFileDescriptor closes a file descriptor and removes it from cache if refCnt reaches 0.
func closeFileDescriptor(fd int) error {
	fdCacheLock.Lock()
	defer fdCacheLock.Unlock()

	for path, entry := range fdCache {
		if entry.fd == fd {
			entry.refCnt--
			if entry.refCnt <= 0 {
				err := entry.file.Close()
				delete(fdCache, path)
				return err
			}
			return nil
		}
	}

	// 如果在缓存中没有找到，直接关闭
	return unix.Close(fd)
}

// applyFadvise applies fadvise to the file using the tracked information.
// offset: 当前操作的偏移量
// length: 累计的操作长度（从 0 开始累加每次操作的大小）
func (t *IOTracker) applyFadvise() error {
	if t.fd < 0 {
		return unix.EBADF
	}

	// 同步并应用 fadvise
	fdCacheLock.RLock()
	var file *os.File
	for _, entry := range fdCache {
		if entry.fd == t.fd {
			file = entry.file
			break
		}
	}
	fdCacheLock.RUnlock()

	if file != nil {
		// 同步文件数据到磁盘
		if err := file.Sync(); err != nil {
			return err
		}
	}

	return unix.Fadvise(t.fd, t.offset, t.length, unix.FADV_DONTNEED)
}
