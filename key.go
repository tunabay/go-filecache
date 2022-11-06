// Copyright (c) 2022 Hirotsuna Mizuno. All rights reserved.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package filecache

import (
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
)

// HashSize is the size, in bytes, of the key hash.
const HashSize = 32

// Hash represents a 32-bytes hash value.
type Hash [HashSize]byte

// Key is the interface implemented by the key type to identify a file cache
// entry.
type Key interface {
	fmt.Stringer

	Hash() Hash
}

// Uint64Key is a wrapper type to attach the Hash method to uint64.
type Uint64Key uint64

// Hash returns a byte sequence in which the LSBs represent the uint64 value
// itself as a hash value.
func (k Uint64Key) Hash() (b Hash) {
	binary.BigEndian.PutUint64(b[HashSize-8:], uint64(k))
	return
}

// String returns the string representation of the uint64 value.
func (k Uint64Key) String() string { return strconv.FormatUint(uint64(k), 10) }

// Uint32Key is a wrapper type to attach the Hash method to uint32.
type Uint32Key uint32

// Hash returns a byte sequence in which the LSBs represent the uint32 value
// itself as a hash value.
func (k Uint32Key) Hash() (b Hash) {
	binary.BigEndian.PutUint32(b[HashSize-4:], uint32(k))
	return
}

// String returns the string representation of the uint32 value.
func (k Uint32Key) String() string { return strconv.FormatUint(uint64(k), 10) }

// StringKey is a wrapper type to attach the Hash method to string.
type StringKey string

// Hash calculates and returns a hash value of the string using SHA-512/256.
func (k StringKey) Hash() Hash { return sha512.Sum512_256([]byte(k)) }

// String returns the string value itself of StringKey.
func (k StringKey) String() string { return string(k) }

// ByteSliceKey is a wrapper type to attach the Hash method to []byte.
type ByteSliceKey []byte

// Hash calculates and returns a hash value of the []byte using SHA-512/256.
func (k ByteSliceKey) Hash() Hash { return sha512.Sum512_256([]byte(k)) }

// String returns the string representation of the []byte key.
func (k ByteSliceKey) String() string { return hex.EncodeToString([]byte(k)) }
