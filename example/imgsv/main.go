// Copyright (c) 2022 Hirotsuna Mizuno. All rights reserved.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"
)

// main is the main function of this example program. A simple image web server
// that serves dynamically generated images over HTTP.
//
// Generating the image takes some time, so the response to the first request is
// a bit delayed. However, subsequent requests with the same parameters will
// return the cached image, resulting in a faster response.
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Parse command parameters.
	listenAddr := ":8080"
	switch {
	case len(os.Args) == 1:
		// use default addr

	case 2 < len(os.Args), strings.HasPrefix(strings.TrimLeft(os.Args[1], "-"), "h"):
		fmt.Fprintf(os.Stderr, "USAGE: %s [ [host]:port ]\n", os.Args[0])
		return

	default:
		listenAddr = os.Args[1]
	}

	// Create and run the image server.
	sv, err := newServer()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: server: %v\n", err)
		return
	}
	go func() {
		if err := sv.serve(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: server: %v\n", err)
			os.Exit(1)
		}
	}()

	// Create and run the HTTP server.
	httpd := &http.Server{
		Addr:           listenAddr,
		Handler:        sv,
		ReadTimeout:    time.Second * 10,
		WriteTimeout:   time.Minute,
		MaxHeaderBytes: 512,
	}
	go func() {
		<-ctx.Done()
		sdctx, sdcancel := context.WithTimeout(context.Background(), time.Second*5)
		defer sdcancel()
		if err := httpd.Shutdown(sdctx); err != nil { //nolint:contextcheck
			fmt.Fprintf(os.Stderr, "ERROR: httpd: %v\n", err)
		}
	}()
	if err := httpd.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "httpd: %v\n", err)
	}
}
