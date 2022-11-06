// Copyright (c) 2022 Hirotsuna Mizuno. All rights reserved.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"sync"
)

// createImage is the callback function that will be called when an image that
// is not in the cache is requested. It receives an image parameter and an
// opened os.File object as arguments. It generates an image and writes it to
// the file. The passed file is automatically closed after return, so there is
// no need to close it here.
//
// The calculation code in this function simply generates a deterministic
// geometric pattern based on the given parameters and output it as a PNG, and
// has nothing to do with caching. The important point is that it takes a long
// time to process and outputs the result to a file. Data written to the file
// will be automatically cached and reused for requests with the same
// parameters.
func createImage(p *imgParam, file *os.File) error {
	// Calculate coefficients from the parameters.
	var (
		w, h = int(p.width), int(p.height)
		tc   = float64(p.twist) * math.Pi / 180
		sc   = float64(p.stripe) / math.Pi
		cc   = float64(p.circleWidth) * .005
		ic   = make([]color.NRGBA, 257)
		bc   float64
	)
	xc, yc, zc := float64(-1), -float64(h)/float64(w), .125/float64(w)
	if w < h {
		xc, yc, zc = -float64(w)/float64(h), -1, .125/float64(h)
	}

	// Prepare colors for the image pixels.
	eo := func(i int) float64 {
		ev := float64(p.color[i]) / 255
		if ev <= .04045 {
			return ev / 12.92
		}
		return math.Pow(math.FMA(ev, 1/1.055, .055/1.055), 2.4)
	}
	const ax, ay, az = .2126755, .71513641, .072188085
	if eo(0)*ax+eo(1)*ay+eo(2)*az+.05 < math.Sqrt(.0525) {
		bc = 255
	}
	c := func(i, t int) byte {
		v := math.FMA(float64(p.color[i])-bc, float64(t)/256, bc)
		return byte(math.Round(v))
	}
	for i := range ic {
		ic[i] = color.NRGBA{R: c(0, i), G: c(1, i), B: c(2, i), A: 0xff}
	}

	// Create image and paint pixels.
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	pc := func(x, y float64) int {
		r := math.Hypot(x, y)
		a := math.Mod(math.Atan2(y, x)+r*tc, math.Pi*2) + math.Pi*2
		s := int(math.Floor(a * sc))
		if r < .05 {
			return s & 1
		}
		for cr := float64(0); cr < 1.5; cr += .25 {
			if math.Abs(r-cr) < cc {
				return s & 1
			}
		}
		return ^s & 1
	}
	paintRow := func(py int) {
		py4 := py << 4
		for px := 0; px < w; px++ {
			px4, n := px<<4, 0
			for v := 0; v < 16; v++ {
				y := math.FMA(float64(py4+v), zc, yc)
				for u := 0; u < 16; u++ {
					n += pc(math.FMA(float64(px4+u), zc, xc), y)
				}
			}
			img.SetNRGBA(px, py, ic[n])
		}
	}
	var wg sync.WaitGroup
	for y := 0; y < h; y++ {
		wg.Add(1)
		go func(py int) {
			defer wg.Done()
			paintRow(py)
		}(y)
	}
	wg.Wait()

	// Encode image as PNG and write to the file.
	if err := png.Encode(file, img); err != nil {
		return fmt.Errorf("failed to encode PNG: %w", err)
	}

	return nil
}
