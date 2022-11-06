// Copyright (c) 2022 Hirotsuna Mizuno. All rights reserved.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package filecache

import "errors"

// ErrInvalidConfig is the error thrown when the passed configuration parameter
// is not valid.
var ErrInvalidConfig = errors.New("invalid config")

// ErrInternal is the error thrown when an internal error occurred.
var ErrInternal = errors.New("internal error")
