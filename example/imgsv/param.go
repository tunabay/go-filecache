// Copyright (c) 2022 Hirotsuna Mizuno. All rights reserved.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"crypto/sha512"
	"encoding/binary"
	"fmt"

	"github.com/tunabay/go-filecache"
)

// imgParam represents the set of parameters to generate image. Since the same
// image is always generated from the same parameter set, the image is cached
// using this imgParam as a key.
type imgParam struct {
	color         [3]byte // RRGGBB value of the key color.
	width, height uint16  // image dimension in pixel, at least 16x16.
	twist         int16   // amount to twist in degrees, can be negative.
	circleWidth   uint16  // line thickness of the concentric circles, 1..16.
	stripe        uint16  // number of radiation stripes, 1..90.
}

// String returns the string representation of the parameter set. It implements
// filecache.Key interface so that it can be used as a cache key.
//
// It is only used for logging and file information, and is not that important.
// Still, it is required to implement the filecache.Key interface. Also not all
// fields need to be included in the return value, as it is not used for hash
// calculation.
func (p *imgParam) String() string {
	return fmt.Sprintf(
		"size=%dx%d, color=#%x, twist=%d, circle-width=%d, stripe=%d",
		p.width,
		p.height,
		p.color,
		p.twist,
		p.circleWidth,
		p.stripe,
	)
}

// Hash computes and returns the hash value of the parameter set using
// SHA-512/256. The same set of parameters will always return the same hash
// value. It implements the filecache.Key interface.
func (p *imgParam) Hash() filecache.Hash {
	b := make([]byte, 13)
	copy(b, p.color[:])
	binary.BigEndian.PutUint16(b[3:], uint16(p.twist))
	binary.BigEndian.PutUint16(b[5:], p.circleWidth)
	binary.BigEndian.PutUint16(b[7:], p.stripe)
	binary.BigEndian.PutUint16(b[9:], p.width)
	binary.BigEndian.PutUint16(b[11:], p.height)

	// Since this is an example, this calculates and returns a SHA-512/256
	// hash value. However, since this data length fits in 32 bytes, it may
	// be better to return the byte string itself that represents the
	// parameter values directly.
	return sha512.Sum512_256(b)
}
