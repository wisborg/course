package main

import (
	"math"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

// The face the map's names, markers and credit are drawn in: the Go font,
// which is inside golang.org/x/image already and has glyphs for the names a
// map of anywhere carries -- a bitmap font drew "Sønder" as "S0nder".
// osmbase's library ships no font on purpose, so every program that draws
// text brings its own.

const baseTextSize = 13

var parsed = sync.OnceValues(func() (*opentype.Font, error) { return opentype.Parse(goregular.TTF) })

var (
	faceMu sync.Mutex
	faces  = map[int]font.Face{}
)

// faceAt is the face at a size in pixels, or nil if the font will not load --
// which draws no text rather than no map.
func faceAt(px float64) font.Face {
	key := int(math.Round(max(px, 1)))
	faceMu.Lock()
	defer faceMu.Unlock()
	if f, ok := faces[key]; ok {
		return f
	}
	ttf, err := parsed()
	if err != nil {
		return nil
	}
	f, err := opentype.NewFace(ttf, &opentype.FaceOptions{Size: float64(key), DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil
	}
	faces[key] = f
	return f
}
