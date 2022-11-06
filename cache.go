// Copyright (c) 2022 Hirotsuna Mizuno. All rights reserved.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package filecache

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/petar/GoLLRB/llrb"
	"github.com/tunabay/go-infounit"
)

// Cache represents the file cache directory.
type Cache[K Key] struct {
	dir        string
	create     CreateFunc[K]
	maxFiles   uint64
	maxSize    infounit.ByteCount
	maxAge     time.Duration
	gcInterval time.Duration

	numFiles     uint64
	totalSize    infounit.ByteCount
	numRequested uint64
	numHit       uint64
	numCreated   uint64
	numFailed    uint64
	numRemoved   uint64

	opMap  map[Hash]*opEntry
	refMap map[Hash]int
	cond   *sync.Cond
	mu     sync.Mutex

	log      Logger
	debugLog bool
}

func (c *Cache[_]) ref(hash Hash) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v := c.refMap[hash]
	c.refMap[hash] = v + 1
}

func (c *Cache[K]) unref(hash Hash) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v := c.refMap[hash]
	if v == 1 {
		delete(c.refMap, hash)
	} else {
		c.refMap[hash] = v - 1
	}
}

// CreateFunc represents a function for file creation. It will be called when
// Get is called for a new key that does not exist in the cache. It should
// create the file using the pre-opened file argument passed. It does not have
// to close the file, while it will be closed automatically after return.
type CreateFunc[K Key] func(K, *os.File) error

// New create a cache with the default configuration.
func New[K Key](dir string, create CreateFunc[K]) (*Cache[K], error) {
	return NewWithConfig[K](
		&Config[K]{
			Dir:      dir,
			Create:   create,
			MaxSize:  infounit.Gigabyte,
			MaxFiles: 512,
			MaxAge:   time.Hour * 24,
		},
	)
}

// NewWithConfig create a cache using the given configuration parameters.
func NewWithConfig[K Key](conf *Config[K]) (*Cache[K], error) {
	switch {
	case conf.Dir == "":
		return nil, fmt.Errorf("%w: empty Dir", ErrInvalidConfig)
	case conf.Create == nil:
		return nil, fmt.Errorf("%w: nil Create", ErrInvalidConfig)
	case conf.MaxAge < 0:
		return nil, fmt.Errorf("%w: negative MaxAge", ErrInvalidConfig)
	case conf.GCInterval < 0:
		return nil, fmt.Errorf("%w: negative GCInterval", ErrInvalidConfig)
	}

	c := &Cache[K]{
		dir:        conf.Dir,
		create:     conf.Create,
		maxFiles:   conf.MaxFiles,
		maxSize:    conf.MaxSize,
		maxAge:     conf.MaxAge,
		gcInterval: conf.GCInterval,

		opMap:  make(map[Hash]*opEntry),
		refMap: make(map[Hash]int),

		log:      conf.Logger,
		debugLog: conf.DebugLog,
	}
	c.cond = sync.NewCond(&c.mu)

	if c.gcInterval == 0 {
		c.gcInterval = defaultGCInterval
	}

	if c.dir == "" {
		c.dir = filepath.Base(os.Args[0])
	}
	if !filepath.IsAbs(c.dir) {
		ucd, err := os.UserCacheDir()
		if err != nil {
			return nil, fmt.Errorf("%s: can not resolve relative cache dir: %w", c.dir, err)
		}
		c.dir = filepath.Join(ucd, c.dir)
	}

	if err := os.MkdirAll(c.dir, 0o0700); err != nil {
		return nil, fmt.Errorf("%s: %w", c.dir, err)
	}
	c.logPrintf("Cache directory: %s", c.dir)

	// read dir, count total size, total files.
	var (
		numRemoved  uint64
		sizeRemoved infounit.ByteCount
	)
	walker := func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			c.logPrintf("%s: Skip unreadable file.", path)
			return fs.SkipDir
		case d.IsDir():
			return nil
		}
		fname := d.Name()
		if len(fname) != HashSize*2 {
			c.logPrintf("%s: Skip unexpected file in cache dir.", fname)
			return nil
		}
		if _, err := hex.DecodeString(fname); err != nil {
			c.logPrintf("%s: Skip unexpected file in cache dir.", fname)
			return nil
		}
		finfo, err := d.Info()
		sz := infounit.ByteCount(finfo.Size())
		if err != nil {
			c.logPrintf("%s: Failed to stat: %v", path, err)
			return nil
		}
		age := time.Since(finfo.ModTime())
		if c.maxAge < age {
			if err := os.Remove(path); err != nil {
				c.logPrintf("%s: Failed to remove expired cache: %v", path, err)
				return nil
			}
			c.logPrintf("%s: Removed expired cache. size=%.1S, age=%v", fname, sz, age)
			numRemoved++
			sizeRemoved += sz
			return nil
		}
		c.numFiles++
		c.totalSize += sz
		c.logDebugf("%s: Cache found. size=%.1S, age=%v", fname, sz, age)

		return nil
	}
	if err := filepath.WalkDir(c.dir, walker); err != nil {
		c.logPrintf("%s: Failed to read cache dir: %v", c.dir, err)
		return nil, fmt.Errorf("%s: failed to read cache dir: %w", c.dir, err)
	}
	if numRemoved != 0 {
		c.logPrintf("Removed %d expired cache files. total=%.1S", numRemoved, sizeRemoved)
	}
	if c.numFiles != 0 {
		c.logPrintf("Found %d cache files. total=%.1S", c.numFiles, c.totalSize)
	}

	return c, nil
}

// Serve serves the Cache instance. It performs find and delete old cache files.
func (c *Cache[K]) Serve(ctx context.Context) error {
	rmCache := func(hash Hash, path string, lastMod time.Time) error {
		c.mu.Lock()
		if _, refed := c.refMap[hash]; refed {
			c.mu.Unlock()
			return nil // concurrently read
		}
		finfo, err := os.Stat(path)
		if err != nil {
			return nil // file disappeared?
		}
		if !lastMod.Equal(finfo.ModTime()) {
			return nil // concurrently accessed
		}
		op := &opEntry{opType: 1, done: make(chan struct{})}
		c.opMap[hash] = op
		c.mu.Unlock()

		if err := os.Remove(path); err != nil {
			c.mu.Lock()
			delete(c.opMap, hash)
			c.mu.Unlock()
			close(op.done)

			return fmt.Errorf("%x: %w", hash[:], err)
		}
		c.logPrintf("%x: Removed.", hash[:]) // successfully removed

		c.mu.Lock()
		delete(c.opMap, hash)
		c.numRemoved++
		c.numFiles--
		c.totalSize -= infounit.ByteCount(finfo.Size())
		c.mu.Unlock()
		close(op.done)

		return nil
	}

	zeroCand := &candidate{}
	for {
		c.mu.Lock()
		for c.numFiles <= c.maxFiles && c.totalSize <= c.maxSize && ctx.Err() == nil {
			c.cond.Wait()
		}
		c.mu.Unlock()
		if err := ctx.Err(); err != nil {
			return nil
		}

		c.logDebugf("Started GC...")

		// build candidates
		var maxCands uint64 = 64
		for c.numFiles+maxCands < c.maxFiles {
			maxCands <<= 1
		}
		tree := llrb.New()

		walker := func(path string, d fs.DirEntry, err error) error {
			switch {
			case err != nil:
				return fs.SkipDir
			case d.IsDir():
				return nil
			}
			fname := d.Name()
			if len(fname) != HashSize*2 {
				return nil
			}
			fhashb, err := hex.DecodeString(fname)
			if err != nil {
				return nil
			}
			var fhash Hash
			copy(fhash[:], fhashb)

			finfo, err := d.Info()
			if err != nil {
				return nil
			}
			if age := time.Since(finfo.ModTime()); c.maxAge < age {
				if err := rmCache(fhash, path, finfo.ModTime()); err != nil {
					c.logPrintf("%s: Failed to remove expired cache: %v", path, err)
				}
				return nil
			}
			cand := &candidate{
				hash:    fhash,
				path:    path,
				lastMod: finfo.ModTime(),
			}
			tree.InsertNoReplace(cand)
			if maxCands < uint64(tree.Len()) {
				tree.DeleteMax()
			}
			return nil
		}
		if err := filepath.WalkDir(c.dir, walker); err != nil {
			continue // failed to read
		}

		candList := make([]*candidate, tree.Len())
		n := 0
		iterator := func(iif llrb.Item) bool {
			candList[n] = iif.(*candidate) //nolint:forcetypeassert
			n++
			return true
		}
		tree.AscendGreaterOrEqual(zeroCand, iterator)

		for _, cand := range candList {
			c.mu.Lock()
			overflow := c.maxFiles < c.numFiles || c.maxSize < c.totalSize
			if !overflow {
				c.mu.Unlock()
				break
			}
			c.mu.Unlock()

			if err := rmCache(cand.hash, cand.path, cand.lastMod); err != nil {
				c.logPrintf("%s: Failed to remove expired cache: %v", cand.path, err)
				continue
			}
		}
		c.logDebugf("GC finished.")

		// wait for the next
		timer := time.NewTimer(c.gcInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil
		case <-timer.C:
		}
	}
}

// candidate represents a candidate file for deletion in the cache directory.
// Among these candidates, those with the oldest lastMod will be deleted in
// order.
type candidate struct {
	hash    Hash
	path    string
	lastMod time.Time
}

// Less compares the lastMod values of the two candidates and reports the
// result.
func (c *candidate) Less(xif llrb.Item) bool {
	x := xif.(*candidate) //nolint:forcetypeassert
	return c.lastMod.Before(x.lastMod)
}

// Get gets the file for the key from the cache. If the file for the specified
// key does not exist in the cache, it will call the CreateFunc to create the
// new file. It returns the file opened for read, cached or not.
//
// The returned file is guaranteed to remain referenced until it is closed and
// not removed from the cache during that time. After using the file, it is the
// caller's responsibility to call the File.Close() method of the returned file.
// Otherwise the file will remain in the cache, and the reference will remain in
// the memory.
func (c *Cache[K]) Get(key K) (*File[K], bool, error) {
	hash := key.Hash()

	c.logDebugf("Get: key=%q", key.String())

	dir, path := c.filePath(hash)

	var (
		created bool
		lastMod time.Time
	)
	for isRetry := false; ; isRetry = true {
		c.mu.Lock()
		if !isRetry {
			c.numRequested++
		}
		op, ok := c.opMap[hash]
		switch {
		case ok && op.opType == 0:
			// concurrently being created
			c.numHit++
			c.mu.Unlock()
			c.logDebugf("Get: File is being created concurrently, waiting for completion...")
			<-op.done
			if op.err != nil {
				return nil, false, op.err
			}
			// file exists, which is just created

		case ok:
			// concurrently being removed
			c.mu.Unlock()
			if isRetry {
				return nil, false, fmt.Errorf("%w: removal twice", ErrInternal)
			}
			c.logDebugf("Get: File is being deleted concurrently, waiting for completion...")
			<-op.done
			continue

		default:
			// no concurrent operation
			if cinfo, err := os.Stat(path); err != nil {
				if !errors.Is(err, fs.ErrNotExist) {
					c.numFailed++
					c.mu.Unlock()
					return nil, false, fmt.Errorf("internal error, stat failed: %w", err)
				}

				// file does not exist
				c.logDebugf("Get: File does not exist, creating...")
				op = &opEntry{done: make(chan struct{})}
				c.opMap[hash] = op
				c.mu.Unlock()

				if err := os.MkdirAll(dir, 0o0700); err != nil {
					op.err = fmt.Errorf("%s: failed to create: %w", dir, err)
					_ = os.Remove(path)
					c.mu.Lock()
					delete(c.opMap, hash)
					c.numFailed++
					c.mu.Unlock()
					close(op.done)

					return nil, false, op.err
				}

				tmpPath := path + ".tmp"

				f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o0644)
				if err != nil {
					op.err = fmt.Errorf("failed to open file: %w", err)
					_ = os.Remove(tmpPath)
					_ = os.Remove(path)
					c.mu.Lock()
					delete(c.opMap, hash)
					c.numFailed++
					c.mu.Unlock()
					close(op.done)

					return nil, false, op.err
				}
				if err := c.create(key, f); err != nil {
					op.err = fmt.Errorf("failed to create file: %w", err)
					_ = f.Close()
					_ = os.Remove(tmpPath)
					_ = os.Remove(path)
					c.mu.Lock()
					delete(c.opMap, hash)
					c.numFailed++
					c.mu.Unlock()
					close(op.done)

					return nil, false, op.err
				}
				_ = f.Close()

				var sz infounit.ByteCount
				finfo, err := os.Stat(tmpPath)
				if err != nil {
					op.err = fmt.Errorf("failed to stat file: %w", err)
					_ = os.Remove(tmpPath)
					_ = os.Remove(path)
					c.mu.Lock()
					delete(c.opMap, hash)
					c.numFailed++
					c.mu.Unlock()
					close(op.done)

					return nil, false, op.err
				}
				sz = infounit.ByteCount(finfo.Size())

				if err := os.Rename(tmpPath, path); err != nil {
					op.err = fmt.Errorf("failed to write file: %w", err)
					_ = os.Remove(tmpPath)
					_ = os.Remove(path)
					c.mu.Lock()
					delete(c.opMap, hash)
					c.numFailed++
					c.mu.Unlock()
					close(op.done)

					return nil, false, op.err
				}

				// file created
				c.logPrintf("Get: File successfully created and cached. size=%d", sz)
				c.mu.Lock()
				c.numFiles++
				c.totalSize += sz
				delete(c.opMap, hash)
				c.cond.Broadcast()
				c.numCreated++
				c.mu.Unlock()
				close(op.done)
				created = true
			} else {
				// file exists
				c.logDebugf("Get: Cache exists.")
				lastMod = cinfo.ModTime()
				tnow := time.Now()
				_ = os.Chtimes(path, tnow, tnow)
				c.numHit++
				c.mu.Unlock()
			}
		}
		break
	}

	osFile, err := os.Open(path) // O_RDONLY
	if err != nil {
		_ = osFile.Close()
		return nil, !created, fmt.Errorf("failed to open: %w", err)
	}
	finfo, err := osFile.Stat()
	if err != nil {
		_ = osFile.Close()
		return nil, !created, fmt.Errorf("failed to stat: %w", err)
	}
	if lastMod.IsZero() {
		lastMod = finfo.ModTime()
	}

	file := &File[K]{
		parent:  c,
		key:     key,
		hash:    hash,
		file:    osFile,
		size:    finfo.Size(),
		lastMod: lastMod,
	}
	c.ref(hash)

	return file, !created, nil
}

// opEntry represents the currently processing operation on a cache entry. When
// accessing an entry, if an another goroutine is processing it, it uses the
// done channel to wait for that processing to complete.
type opEntry struct {
	opType uint8         // 0: creating, 1: removing
	done   chan struct{} // closed when operation done
	err    error
}

// filePath returns the full path of the cache file corresponding to the given
// hash value.
func (c *Cache[_]) filePath(hash Hash) (dir, path string) {
	dir = filepath.Join(c.dir, b2hex(hash[HashSize-1]), b2hex(hash[HashSize-2]))
	path = filepath.Join(dir, hashHex(hash))
	return
}

// Status represents the cache status and statistics.
type Status struct {
	NumFiles     uint64             // number of files currently in cache.
	TotalSize    infounit.ByteCount // total size of files currently in cache.
	NumRequested uint64             // total number of files requested.
	NumHit       uint64             // total number of cache hits.
	NumCreated   uint64             // total number of newly created cache files.
	NumFailed    uint64             // total number of operation failures.
	NumRemoved   uint64             // total number of removed cache files.
	NumOps       int                // number of operations currently being processed.
	NumRefs      int                // number of currently referenced cache files.
}

// String returns the string representation of Status.
func (s Status) String() string {
	return fmt.Sprintf(
		"files=%d, size=%.1S, req=%d, hit=%d, new=%d, fail=%d, del=%d, op=%d, ref=%d",
		s.NumFiles,
		s.TotalSize,
		s.NumRequested,
		s.NumHit,
		s.NumCreated,
		s.NumFailed,
		s.NumRemoved,
		s.NumOps,
		s.NumRefs,
	)
}

// Status returns the current cache status and statistics.
func (c *Cache[_]) Status() *Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return &Status{
		NumFiles:     c.numFiles,
		TotalSize:    c.totalSize,
		NumRequested: c.numRequested,
		NumHit:       c.numHit,
		NumCreated:   c.numCreated,
		NumFailed:    c.numFailed,
		NumRemoved:   c.numRemoved,
		NumOps:       len(c.opMap),
		NumRefs:      len(c.refMap),
	}
}

// logPrefix returns the prefix string for log messages, according to the
// current configuration.
func (c *Cache[_]) logPrefix() string {
	if !c.debugLog {
		return ""
	}
	if _, file, line, ok := runtime.Caller(2); ok {
		return fmt.Sprintf("%s:%d:", filepath.Base(file), line)
	}
	return "(unknown):"
}

// logPrintf outputs a log message according to the current configuration.
func (c *Cache[_]) logPrintf(format string, v ...any) {
	if c.log == nil {
		return
	}
	s := make([]string, 0, 2)
	if prefix := c.logPrefix(); prefix != "" {
		s = append(s, prefix)
	}
	s = append(s, fmt.Sprintf(format, v...))

	c.log.FileCacheLog(strings.Join(s, " "))
}

// logDebugf outputs a debug log message according to the current configuration.
func (c *Cache[_]) logDebugf(format string, v ...any) {
	if c.log == nil || !c.debugLog {
		return
	}

	s := make([]string, 0, 2)
	if prefix := c.logPrefix(); prefix != "" {
		s = append(s, prefix)
	}
	s = append(s, fmt.Sprintf(format, v...))

	c.log.FileCacheLog(strings.Join(s, " "))
}
