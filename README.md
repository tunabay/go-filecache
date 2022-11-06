# go-filecache

[![Go Reference](https://pkg.go.dev/badge/github.com/tunabay/go-filecache.svg)](https://pkg.go.dev/github.com/tunabay/go-filecache)
[![MIT License](http://img.shields.io/badge/license-MIT-blue.svg?style=flat)](LICENSE)


## Overview

Package filecache provides a LRU file caching mechanism to cache resources to
the local disk that take a long time to generate or download from the network.

The creation process runs only once even if multiple go-routines concurrently
request for a key that does not exist in the cache.


## Documentation

- Read the [documentation](https://pkg.go.dev/github.com/tunabay/go-filecache).


## License

go-filecache is available under the MIT license. See the [LICENSE](LICENSE) file
for more information.
