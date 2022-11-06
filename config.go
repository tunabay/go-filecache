// Copyright (c) 2022 Hirotsuna Mizuno. All rights reserved.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package filecache

import (
	"time"

	"github.com/tunabay/go-infounit"
)

// Config represents the parameters to configure Cache creation.
type Config[K Key] struct {
	// The path to the directory for cache files. It should be a dedicated
	// directory used exclusively for this cache. The directory will be
	// automatically created if it does not exist. Both absolute and
	// relative paths are allowed. A relative path is treated as relative
	// from the user-specific cache directory returned by os.UserCacheDir().
	// If it is empty, use the program name directory.
	Dir string

	// The callback function that is called when a not-cached resource is
	// requested.
	Create CreateFunc[K]

	// The upper limit on the number of files that can be cached. Zero
	// value means unlimited. When more than this number of files are
	// cached, the oldest files will be removed. Note that more than this
	// number of files may be cached temporarily.
	MaxFiles uint64

	// The limit on the total size of files that can be cached. Zero
	// value means unlimited. When more than this total size of files are
	// cached, the oldest files will be removed. Note that more than this
	// size of files may be cached temporarily. There is no guarantee that
	// more disk space than this will not be used.
	MaxSize infounit.ByteCount

	// The maximum age of cache files. Note that it is the time since
	// last access, not the time since creation. Also the cache is not
	// removed immediately after this age. It is still possible that an
	// aged cache file will continue to be hit and reused. Zero value
	// means unlimited.
	MaxAge time.Duration

	// The interval between GC processing to find and remove old cache
	// files that exceed the configured limits.
	GCInterval time.Duration

	// If not nil, Cache outputs log messages to this Logger object.
	Logger Logger

	// If true, Cache outputs debug log messages. Only effective if
	// Logger is not nil.
	DebugLog bool
}

// defaultGCInterval defines the default value for Config.GCInterval.
const defaultGCInterval = time.Minute

// Logger is the interface implemented to receive log messages from the running
// Cache instance.
type Logger interface {
	FileCacheLog(string)
}
