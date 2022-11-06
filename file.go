// Copyright (c) 2022 Hirotsuna Mizuno. All rights reserved.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package filecache

import (
	"io/fs"
	"os"
	"time"
)

// File represents the cached file returned by Get() method.
type File[K Key] struct {
	parent  *Cache[K]
	key     K
	hash    Hash
	file    *os.File
	size    int64
	lastMod time.Time
}

// Name returns the string representation of the associated key.
func (f *File[_]) Name() string { return f.key.String() }

// Read implements io.Reader interface.
func (f *File[_]) Read(b []byte) (int, error) {
	return f.file.Read(b) //nolint:wrapcheck
}

// ReadAt implements io.ReaderAt interface.
func (f *File[_]) ReadAt(b []byte, off int64) (int, error) {
	return f.file.ReadAt(b, off) //nolint:wrapcheck
}

// Seek implements io.Seeker interface.
func (f *File[_]) Seek(offset int64, whence int) (int64, error) {
	return f.file.Seek(offset, whence) //nolint:wrapcheck
}

// Close implements io.Closer interface.
func (f *File[_]) Close() error {
	f.parent.unref(f.hash)

	return f.file.Close() //nolint:wrapcheck
}

// OSFile returns the underlying os.File object. Use with caution, as all writes
// will fail and operations such as deleting, renaming or closing the file will
// cause unexpected results. Only use if it is required to pass the file to a
// package that accepts only os.File or the file descriptor. Also use File.Close
// instead of os.File.Close even if this method is called.
func (f *File[_]) OSFile() *os.File { return f.file }

// Stat returns the file information.
func (f *File[K]) Stat() (os.FileInfo, error) {
	return &FileInfo[K]{
		key:     f.key,
		size:    f.size,
		lastMod: f.lastMod,
	}, nil
}

// FileInfo implements fs.FileInfo interface.
type FileInfo[K Key] struct {
	key     K
	size    int64
	lastMod time.Time
}

// Name returns the string representation of the key. Note that it is not the
// underlying file path.
func (i *FileInfo[K]) Name() string { return i.key.String() }

// Size returns the file size in byte.
func (i *FileInfo[_]) Size() int64 { return i.size }

// Mode always returns 0400.
func (i *FileInfo[_]) Mode() fs.FileMode { return 0o0400 }

// ModTime returns the last access time or created time of the cache entry.
func (i *FileInfo[_]) ModTime() time.Time { return i.lastMod }

// IsDir always returns false, since a directory can not be cached.
func (*FileInfo[_]) IsDir() bool { return false }

// Sys returns the associated key.
func (i *FileInfo[_]) Sys() any { return i.key }
