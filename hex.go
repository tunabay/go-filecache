// Copyright (c) 2022 Hirotsuna Mizuno. All rights reserved.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package filecache

import (
	"encoding/hex"
)

// b2hex converts a byte into a hex two letters string.
func b2hex(b byte) string { return hex.EncodeToString([]byte{b}) }

// hashHex returns the hex representation of the hash.
func hashHex(hash Hash) string { return hex.EncodeToString(hash[:]) }
