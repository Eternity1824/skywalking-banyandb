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

package fadvise

import (
	"bufio"
	"io"
	"os"
	"sync/atomic"

	"github.com/apache/skywalking-banyandb/pkg/fs"
)

const defaultIOSize = 256 * 1024

// File is a wrapper around os.File that adds fadvise functionality.
type File struct {
	file         *os.File
	bytesRead    uint64
	bytesWritten uint64
	isLargeFile  bool
	threshold    int64
}

// NewFile creates a new File.
func NewFile(file *os.File, path string) fs.File {
	return &File{
		file:      file,
		threshold: 1024 * 1024 * 100, // 100MB
	}
}

// NewFileWithThreshold creates a new File with custom threshold.
func NewFileWithThreshold(file *os.File, path string, threshold int64) fs.File {
	return &File{
		file:      file,
		threshold: threshold,
	}
}

// Read implements File interface.
func (f *File) Read(offset int64, buffer []byte) (n int, err error) {
	n, err = f.file.ReadAt(buffer, offset)
	if n > 0 {
		atomic.AddUint64(&f.bytesRead, uint64(n))
		if f.isLargeFile {
			applyFadviseToFd(int(f.file.Fd()), int64(f.bytesRead), int64(n))
		}
	}
	return n, err
}

// Readv implements File interface.
func (f *File) Readv(offset int64, iov *[][]byte) (n int, err error) {
	var size int
	for _, buffer := range *iov {
		rsize, err := f.file.ReadAt(buffer, offset)
		if err != nil {
			return size, err
		}
		size += rsize
		offset += int64(rsize)
		atomic.AddUint64(&f.bytesRead, uint64(rsize))
		if f.isLargeFile {
			applyFadviseToFd(int(f.file.Fd()), int64(f.bytesRead), int64(rsize))
		}
	}
	return size, nil
}

// Write implements File interface.
func (f *File) Write(buffer []byte) (n int, err error) {
	n, err = f.file.Write(buffer)
	if n > 0 {
		atomic.AddUint64(&f.bytesWritten, uint64(n))
		if f.isLargeFile {
			applyFadviseToFd(int(f.file.Fd()), int64(f.bytesWritten), int64(n))
		}
	}
	return n, err
}

// Writev implements File interface.
func (f *File) Writev(iov *[][]byte) (n int, err error) {
	var size int
	for _, buffer := range *iov {
		wsize, err := f.file.Write(buffer)
		if err != nil {
			return size, err
		}
		size += wsize
		atomic.AddUint64(&f.bytesWritten, uint64(wsize))
		if f.isLargeFile {
			applyFadviseToFd(int(f.file.Fd()), int64(f.bytesWritten), int64(wsize))
		}
	}
	return size, nil
}

// Size implements File interface.
func (f *File) Size() (int64, error) {
	info, err := f.file.Stat()
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// Path implements File interface.
func (f *File) Path() string {
	return f.file.Name()
}

// Close implements File interface.
func (f *File) Close() error {
	return f.file.Close()
}

// Fd implements File interface.
func (f *File) Fd() int {
	return int(f.file.Fd())
}

// Offset implements File interface.
func (f *File) Offset() int64 {
	offset, _ := f.file.Seek(0, io.SeekCurrent)
	return offset
}

// SequentialRead implements File interface.
func (f *File) SequentialRead() fs.SeqReader {
	reader := generateReader(f.file)
	return &SeqReader{
		reader:      reader,
		fileName:    f.file.Name(),
		bytesRead:   &f.bytesRead,
		isLargeFile: f.isLargeFile,
		threshold:   f.threshold,
		fd:          int(f.file.Fd()),
		offset:      0,
	}
}

// SequentialWrite implements File interface.
func (f *File) SequentialWrite() fs.SeqWriter {
	writer := generateWriter(f.file)
	return &SeqWriter{
		writer:       writer,
		fileName:     f.file.Name(),
		bytesWritten: &f.bytesWritten,
		isLargeFile:  f.isLargeFile,
		threshold:    f.threshold,
		fd:           int(f.file.Fd()),
		offset:       0,
	}
}

// SeqReader implements SeqReader interface with fadvise functionality.
type SeqReader struct {
	reader      io.Reader
	fileName    string
	bytesRead   *uint64
	isLargeFile bool
	threshold   int64
	fd          int
	offset      int64
}

// Read implements SeqReader interface.
func (r *SeqReader) Read(p []byte) (n int, err error) {
	n, err = r.reader.Read(p)
	if n > 0 {
		atomic.AddUint64(r.bytesRead, uint64(n))
		r.offset += int64(n)
		if r.isLargeFile {
			applyFadviseToFd(r.fd, int64(*r.bytesRead), int64(n))
		}
	}
	return n, err
}

// Path implements SeqReader interface.
func (r *SeqReader) Path() string {
	return r.fileName
}

// Close implements SeqReader interface.
func (r *SeqReader) Close() error {
	if closer, ok := r.reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// Fd implements SeqReader interface.
func (r *SeqReader) Fd() int {
	return r.fd
}

// Offset implements SeqReader interface.
func (r *SeqReader) Offset() int64 {
	return r.offset
}

// SeqWriter implements SeqWriter interface with fadvise functionality.
type SeqWriter struct {
	writer       io.Writer
	fileName     string
	bytesWritten *uint64
	isLargeFile  bool
	threshold    int64
	fd           int
	offset       int64
}

// Write implements SeqWriter interface.
func (w *SeqWriter) Write(p []byte) (n int, err error) {
	n, err = w.writer.Write(p)
	if n > 0 {
		atomic.AddUint64(w.bytesWritten, uint64(n))
		w.offset += int64(n)
		if w.isLargeFile {
			applyFadviseToFd(w.fd, int64(*w.bytesWritten), int64(n))
		}
	}
	return n, err
}

// Path implements SeqWriter interface.
func (w *SeqWriter) Path() string {
	return w.fileName
}

// Close implements SeqWriter interface.
func (w *SeqWriter) Close() error {
	if closer, ok := w.writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// Fd implements SeqWriter interface.
func (w *SeqWriter) Fd() int {
	return w.fd
}

// Offset implements SeqWriter interface.
func (w *SeqWriter) Offset() int64 {
	return w.offset
}

// applyFadviseToFd is a function variable that can be replaced with platform-specific implementations
var applyFadviseToFd func(fd int, offset, length int64) error

func init() {
	// 默认实现为空操作
	applyFadviseToFd = func(fd int, offset, length int64) error {
		return nil
	}
}

func generateReader(f *os.File) *bufio.Reader {
	return bufio.NewReaderSize(f, defaultIOSize)
}

func generateWriter(f *os.File) *bufio.Writer {
	return bufio.NewWriterSize(f, defaultIOSize)
}
