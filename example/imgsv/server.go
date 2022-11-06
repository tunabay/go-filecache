// Copyright (c) 2022 Hirotsuna Mizuno. All rights reserved.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"context"
	"embed"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/tunabay/go-filecache"
	"github.com/tunabay/go-infounit"
)

// cacheDir is the path to the cache directory.
const cacheDir = "/tmp/go-filecache-example"

// server represents the example image server. It holds one filecache.Cache
// instance.
type server struct {
	cache *filecache.Cache[*imgParam]
}

// newServer creates an image server instance.
func newServer() (*server, error) {
	sv := &server{}
	cacheConf := &filecache.Config[*imgParam]{
		Dir:        cacheDir,
		Create:     createImage,
		MaxFiles:   16,
		MaxSize:    infounit.Megabyte * 2,
		MaxAge:     time.Minute * 10,
		GCInterval: time.Minute,
		Logger:     sv,
		DebugLog:   true,
	}
	cache, err := filecache.NewWithConfig[*imgParam](cacheConf)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache: %w", err)
	}
	sv.cache = cache

	return sv, nil
}

// FileCacheLog implements filecache.Logger to receive log messages from the
// filecache package.
func (sv *server) FileCacheLog(line string) {
	fmt.Fprintf(os.Stderr, "filecache: %s\n", line)
}

// serve serves the image server. It calls filecache.Cache.Serve to perform its
// expiration process.
func (sv *server) serve(ctx context.Context) error {
	// Log the cache status every 30 seconds.
	go func() {
		ticker := time.NewTicker(time.Second * 30)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			cstat := sv.cache.Status()
			fmt.Fprintf(os.Stderr, "cache status: %v\n", cstat)
		}
	}()

	if err := sv.cache.Serve(ctx); err != nil {
		return fmt.Errorf("cache: %w", err)
	}

	return nil
}

// ServeHTTP responds to incoming HTTP requests. It extracts the image
// parameter set from the URL requested, and use it as a key to lookup the cache
// for the image.
//
// It immediately sends the image to the client if the cached image exists.
// If the cache does not exist, the filecache.Cache calls the callback function
// createImage() to generate the image, and then returns the image as if the
// cache already exists.
func (sv *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	errf := func(code int, format string, v ...any) {
		b := []byte(fmt.Sprintf(format, v...) + "\n")
		w.Header().Add("Content-Type", "text/plain")
		w.Header().Add("Content-Length", strconv.FormatInt(int64(len(b)), 10))
		w.WriteHeader(code)
		if _, err := w.Write(b); err != nil {
			fmt.Fprintf(os.Stderr, "WARN: ResponseWriter.Write: %v\n", err)
		}
	}

	// Check the request.
	switch {
	case r.Method != http.MethodGet:
		errf(http.StatusMethodNotAllowed, "Method %s not allowed.", r.Method)
		return

	case r.URL.Path == "/":
		if err := sv.serveIndex(w, r); err != nil {
			fmt.Fprintf(os.Stderr, "WARN: index.html: %v\n", err)
		}
		return

	case r.URL.Path == "/image.png":
	case r.URL.Path == "/favicon.ico":

	default:
		errf(http.StatusNotFound, "Resource %s not found.", r.URL.Path)
		return
	}
	fmt.Fprintf(os.Stderr, "REQUEST: %v\n", r.URL)

	// Parse parameters in the query string.
	param := &imgParam{
		width:       1280,
		height:      720,
		color:       [3]byte{0x00, 0x99, 0x00},
		twist:       120,
		circleWidth: 12,
		stripe:      8,
	}
	qvals := r.URL.Query()
	if s := qvals.Get("width"); s != "" {
		v, err := strconv.ParseUint(s, 10, 16)
		switch {
		case err != nil:
			errf(http.StatusNotFound, "Invalid width %q: %v", s, err)
			return
		case v < 16:
			errf(http.StatusNotFound, "Too small width %d, at least 16", v)
			return
		}
		param.width = uint16(v)
	}
	if s := qvals.Get("height"); s != "" {
		v, err := strconv.ParseUint(s, 10, 16)
		switch {
		case err != nil:
			errf(http.StatusNotFound, "Invalid height %q: %v", s, err)
			return
		case v < 16:
			errf(http.StatusNotFound, "Too small height %d, at least 16", v)
			return
		}
		param.height = uint16(v)
	}
	if s := qvals.Get("color"); s != "" {
		v, err := hex.DecodeString(s)
		switch {
		case err != nil:
			errf(http.StatusNotFound, "Invalid color %q: %v", s, err)
			return
		case len(v) != 3:
			errf(http.StatusNotFound, "Invalid color %q", s)
			return
		}
		copy(param.color[:], v)
	}
	if s := qvals.Get("twist"); s != "" {
		v, err := strconv.ParseInt(s, 10, 16)
		if err != nil {
			errf(http.StatusNotFound, "Invalid twist %q: %v", s, err)
			return
		}
		param.twist = int16(v)
	}
	if s := qvals.Get("circle-width"); s != "" {
		v, err := strconv.ParseUint(s, 10, 16)
		switch {
		case err != nil:
			errf(http.StatusNotFound, "Invalid circle-width %q: %v", s, err)
			return
		case v == 0, 16 < v:
			errf(http.StatusNotFound, "Invalid circle-width: %v, must be 1..16", v)
			return
		}
		param.circleWidth = uint16(v)
	}
	if s := qvals.Get("stripe"); s != "" {
		v, err := strconv.ParseUint(s, 10, 16)
		switch {
		case err != nil:
			errf(http.StatusNotFound, "Invalid stripe %q: %v", s, err)
			return
		case v == 0, 90 < v:
			errf(http.StatusNotFound, "Invalid stripe %v, must be 1..90", v)
			return
		}
		param.stripe = uint16(v)
	}
	if r.URL.Path == "/favicon.ico" {
		param.width = 64
		param.height = 64
		param.color = [3]byte{0xcc, 0x00, 0x00}
		param.twist = 15
		param.circleWidth = 12
		param.stripe = 4
	}

	startedAt := time.Now()

	file, cached, err := sv.cache.Get(param)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARN: Cache.Get: %v\n", err)
		return
	}
	// IMPORTANT: It's the caller's responsibility to call the Close()
	// method of the file returned by Get().
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "WARN: File.Close: %v\n", err)
		}
	}()

	elapsed := time.Since(startedAt)

	finfo, _ := file.Stat()
	w.Header().Add("Content-Length", strconv.FormatInt(finfo.Size(), 10))
	w.Header().Add("Content-Type", "image/png")
	if _, err := io.Copy(w, file); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: io.Copy: %v\n", err)
		return
	}

	tag := "newly created"
	if cached {
		tag = "cached"
	}
	fmt.Fprintf(os.Stderr, "Served [%s] %s (elapsed %v)\n", finfo.Name(), tag, elapsed)
}

//go:embed index.html
var efs embed.FS

func (sv *server) serveIndex(w http.ResponseWriter, _ *http.Request) error {
	file, err := efs.Open("index.html")
	if err != nil {
		return fmt.Errorf("index.html: %w", err)
	}
	defer file.Close()

	fstat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("index.html: %w", err)
	}

	w.Header().Add("Content-Length", strconv.FormatInt(fstat.Size(), 10))
	w.Header().Add("Content-Type", "text/html")
	if _, err := io.Copy(w, file); err != nil {
		return fmt.Errorf("index.html: %w", err)
	}

	return nil
}
