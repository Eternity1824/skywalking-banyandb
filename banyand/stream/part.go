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

package stream

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"sync/atomic"

	"github.com/apache/skywalking-banyandb/api/common"
	"github.com/apache/skywalking-banyandb/banyand/fadvis"
	"github.com/apache/skywalking-banyandb/banyand/internal/storage"
	"github.com/apache/skywalking-banyandb/pkg/bytes"
	"github.com/apache/skywalking-banyandb/pkg/fs"
	"github.com/apache/skywalking-banyandb/pkg/logger"
	"github.com/apache/skywalking-banyandb/pkg/pool"
)

const (
	metadataFilename               = "metadata.json"
	primaryFilename                = "primary.bin"
	metaFilename                   = "meta.bin"
	timestampsFilename             = "timestamps.bin"
	elementIndexFilename           = "idx"
	tagFamiliesMetadataFilenameExt = ".tfm"
	tagFamiliesFilenameExt         = ".tf"
)

// Functions SetMemoryProtector and SetFadvisThreshold have been moved to fadvis_adapt.go

type part struct {
	primary              fs.Reader
	timestamps           fs.Reader
	fileSystem           fs.FileSystem
	tagFamilyMetadata    map[string]fs.Reader
	tagFamilies          map[string]fs.Reader
	path                 string
	primaryBlockMetadata []primaryBlockMetadata
	partMetadata         partMetadata
}

func (p *part) close() {
	fs.MustClose(p.primary)
	fs.MustClose(p.timestamps)
	for _, tf := range p.tagFamilies {
		fs.MustClose(tf)
	}
	for _, tfh := range p.tagFamilyMetadata {
		fs.MustClose(tfh)
	}
}

func (p *part) String() string {
	return fmt.Sprintf("part %d", p.partMetadata.ID)
}

func openMemPart(mp *memPart) *part {
	var p part
	p.partMetadata = mp.partMetadata

	p.primaryBlockMetadata = mustReadPrimaryBlockMetadata(p.primaryBlockMetadata[:0], &mp.meta)

	// Open data files
	p.primary = &mp.primary
	p.timestamps = &mp.timestamps
	if mp.tagFamilies != nil {
		p.tagFamilies = make(map[string]fs.Reader)
		p.tagFamilyMetadata = make(map[string]fs.Reader)
		for name, tf := range mp.tagFamilies {
			p.tagFamilies[name] = tf
			p.tagFamilyMetadata[name] = mp.tagFamilyMetadata[name]
		}
	}
	return &p
}

type memPart struct {
	tagFamilyMetadata map[string]*bytes.Buffer
	tagFamilies       map[string]*bytes.Buffer
	meta              bytes.Buffer
	primary           bytes.Buffer
	timestamps        bytes.Buffer
	partMetadata      partMetadata
}

func (mp *memPart) mustCreateMemTagFamilyWriters(name string) (fs.Writer, fs.Writer) {
	if mp.tagFamilies == nil {
		mp.tagFamilies = make(map[string]*bytes.Buffer)
		mp.tagFamilyMetadata = make(map[string]*bytes.Buffer)
	}
	tf, ok := mp.tagFamilies[name]
	tfh := mp.tagFamilyMetadata[name]
	if ok {
		tf.Reset()
		tfh.Reset()
		return tfh, tf
	}
	mp.tagFamilies[name] = &bytes.Buffer{}
	mp.tagFamilyMetadata[name] = &bytes.Buffer{}
	return mp.tagFamilyMetadata[name], mp.tagFamilies[name]
}

func (mp *memPart) reset() {
	mp.partMetadata.reset()
	mp.meta.Reset()
	mp.primary.Reset()
	mp.timestamps.Reset()
	if mp.tagFamilies != nil {
		for _, tf := range mp.tagFamilies {
			tf.Reset()
		}
	}
	if mp.tagFamilyMetadata != nil {
		for _, tfh := range mp.tagFamilyMetadata {
			tfh.Reset()
		}
	}
}

func (mp *memPart) mustInitFromElements(es *elements) {
	mp.reset()

	if len(es.timestamps) == 0 {
		return
	}

	sort.Sort(es)

	bsw := generateBlockWriter()
	bsw.MustInitForMemPart(mp)
	var sidPrev common.SeriesID
	uncompressedBlockSizeBytes := uint64(0)
	var indexPrev int
	for i := range es.timestamps {
		sid := es.seriesIDs[i]
		if sidPrev == 0 {
			sidPrev = sid
		}

		if uncompressedBlockSizeBytes >= maxUncompressedBlockSize || sid != sidPrev {
			bsw.MustWriteElements(sidPrev, es.timestamps[indexPrev:i], es.elementIDs[indexPrev:i], es.tagFamilies[indexPrev:i])
			sidPrev = sid
			indexPrev = i
			uncompressedBlockSizeBytes = 0
		}
		uncompressedBlockSizeBytes += uncompressedElementSizeBytes(i, es)
	}
	bsw.MustWriteElements(sidPrev, es.timestamps[indexPrev:], es.elementIDs[indexPrev:], es.tagFamilies[indexPrev:])
	bsw.Flush(&mp.partMetadata)
	releaseBlockWriter(bsw)
}

func (mp *memPart) mustFlush(fileSystem fs.FileSystem, path string) {
	fileSystem.MkdirPanicIfExist(path, storage.DirPerm)

	// Skip metadata index directory
	if filepath.Base(path) == elementIndexFilename {
		return
	}

	// Flush all data files
	fs.MustFlush(fileSystem, mp.meta.Buf, filepath.Join(path, metaFilename), storage.FilePerm)
	fs.MustFlush(fileSystem, mp.primary.Buf, filepath.Join(path, primaryFilename), storage.FilePerm)
	fs.MustFlush(fileSystem, mp.timestamps.Buf, filepath.Join(path, timestampsFilename), storage.FilePerm)
	for name, tf := range mp.tagFamilies {
		fs.MustFlush(fileSystem, tf.Buf, filepath.Join(path, name+tagFamiliesFilenameExt), storage.FilePerm)
	}
	for name, tfh := range mp.tagFamilyMetadata {
		fs.MustFlush(fileSystem, tfh.Buf, filepath.Join(path, name+tagFamiliesMetadataFilenameExt), storage.FilePerm)
	}

	mp.partMetadata.mustWriteMetadata(fileSystem, path)

	// Sync to disk
	fileSystem.SyncPath(path)

	// Apply fadvis to large files
	primaryPath := filepath.Join(path, primaryFilename)
	fadvis.ApplyIfLarge(primaryPath)

	timestampsPath := filepath.Join(path, timestampsFilename)
	fadvis.ApplyIfLarge(timestampsPath)

	for name := range mp.tagFamilies {
		fieldPath := filepath.Join(path, name+tagFamiliesFilenameExt)
		fadvis.ApplyIfLarge(fieldPath)
	}
}

func uncompressedElementSizeBytes(index int, es *elements) uint64 {
	// 8 bytes for timestamp
	// 8 bytes for elementID
	n := uint64(8 + 8)
	for i := range es.tagFamilies[index] {
		n += uint64(len(es.tagFamilies[index][i].tag))
		for j := range es.tagFamilies[index][i].values {
			n += uint64(es.tagFamilies[index][i].values[j].size())
		}
	}
	return n
}

func generateMemPart() *memPart {
	v := memPartPool.Get()
	if v == nil {
		return &memPart{}
	}
	return v
}

func releaseMemPart(mp *memPart) {
	mp.reset()
	memPartPool.Put(mp)
}

var memPartPool = pool.Register[*memPart]("stream-memPart")

type partWrapper struct {
	mp        *memPart
	p         *part
	ref       int32
	removable atomic.Bool
}

func newPartWrapper(mp *memPart, p *part) *partWrapper {
	return &partWrapper{mp: mp, p: p, ref: 1}
}

func (pw *partWrapper) incRef() {
	atomic.AddInt32(&pw.ref, 1)
}

func (pw *partWrapper) decRef() {
	n := atomic.AddInt32(&pw.ref, -1)
	if n > 0 {
		return
	}
	if pw.mp != nil {
		releaseMemPart(pw.mp)
		pw.mp = nil
		pw.p = nil
		return
	}
	pw.p.close()
	if pw.removable.Load() && pw.p.fileSystem != nil {
		go func(pw *partWrapper) {
			pw.p.fileSystem.MustRMAll(pw.p.path)
		}(pw)
	}
}

func (pw *partWrapper) ID() uint64 {
	return pw.p.partMetadata.ID
}

func (pw *partWrapper) String() string {
	if pw.mp != nil {
		return fmt.Sprintf("mem part %v", pw.mp.partMetadata)
	}
	return fmt.Sprintf("part %v", pw.p.partMetadata)
}

func mustOpenFilePart(id uint64, root string, fileSystem fs.FileSystem) *part {
	var p part
	partPath := partPath(root, id)
	p.path = partPath
	p.fileSystem = fileSystem
	p.partMetadata.mustReadMetadata(fileSystem, partPath)
	p.partMetadata.ID = id

	metaPath := path.Join(partPath, metaFilename)
	pr := mustOpenReader(metaPath, fileSystem)
	p.primaryBlockMetadata = mustReadPrimaryBlockMetadata(p.primaryBlockMetadata[:0], pr)
	fs.MustClose(pr)

	p.primary = mustOpenReader(path.Join(partPath, primaryFilename), fileSystem)
	p.timestamps = mustOpenReader(path.Join(partPath, timestampsFilename), fileSystem)
	ee := fileSystem.ReadDir(partPath)
	for _, e := range ee {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) == tagFamiliesMetadataFilenameExt {
			if p.tagFamilyMetadata == nil {
				p.tagFamilyMetadata = make(map[string]fs.Reader)
			}
			p.tagFamilyMetadata[removeExt(e.Name(), tagFamiliesMetadataFilenameExt)] = mustOpenReader(path.Join(partPath, e.Name()), fileSystem)
		}
		if filepath.Ext(e.Name()) == tagFamiliesFilenameExt {
			if p.tagFamilies == nil {
				p.tagFamilies = make(map[string]fs.Reader)
			}
			p.tagFamilies[removeExt(e.Name(), tagFamiliesFilenameExt)] = mustOpenReader(path.Join(partPath, e.Name()), fileSystem)
		}
	}
	return &p
}

func mustOpenReader(name string, fileSystem fs.FileSystem) fs.Reader {
	f, err := fileSystem.OpenFile(name)
	if err != nil {
		logger.Panicf("cannot open %q: %s", name, err)
	}
	return f
}

func removeExt(nameWithExt, ext string) string {
	return nameWithExt[:len(nameWithExt)-len(ext)]
}

func partPath(root string, epoch uint64) string {
	return filepath.Join(root, partName(epoch))
}

func partName(epoch uint64) string {
	return fmt.Sprintf("%016x", epoch)
}

// mustFlush syncs the directory and applies fadvis to large files if needed
func (p *part) mustFlush() {
	fs := p.fileSystem

	// Create directories if needed
	fs.MkdirIfNotExist(p.path, 0o750)

	// Initialize file path variables
	primaryFile := filepath.Join(p.path, primaryFilename)
	timestampsFile := filepath.Join(p.path, timestampsFilename)

	// In the part struct, primary, timestamps, and tagFamilies fields are of fs.Reader type
	// Unlike bytes.Buffer in memPart, they cannot be flushed directly
	// This method is mainly used for syncing directories and applying fadvis,
	// rather than writing memory data to disk like memPart.mustFlush

	// Sync the directory to ensure all files are flushed to disk
	fs.SyncPath(p.path)

	// Apply fadvis to large files to improve file system performance
	if _, err := os.Stat(primaryFile); err == nil && p.primary != nil {
		fadvis.ApplyIfLarge(primaryFile)
	}

	if _, err := os.Stat(timestampsFile); err == nil && p.timestamps != nil {
		fadvis.ApplyIfLarge(timestampsFile)
	}

	for name, _ := range p.tagFamilies {
		tfFile := filepath.Join(p.path, name+tagFamiliesFilenameExt)
		if _, err := os.Stat(tfFile); err == nil {
			fadvis.ApplyIfLarge(tfFile)
		}
	}
}
