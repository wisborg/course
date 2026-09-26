package main

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"

	"github.com/wisborg/osmbase/render"
)

// The faces a map's names, markers and credit are drawn in.
//
// The Go font first: it is inside golang.org/x/image already, is made for
// screens, and has Latin, Greek and Cyrillic. It has nothing else, and a map
// of a flight over Thailand and India drew every name there as boxes. So each
// face is a render.Fallback: the Go font, then any fonts given with --font,
// then whichever of a few well-known system fonts this machine has -- each
// letter from the first of them that can write it. osmbase's library ships no
// font on purpose, so every program that draws text brings its own.

const baseTextSize = 13

// systemFontPaths is systemFonts, or none, for a test that must not depend on
// the fonts of the machine it runs on.
var systemFontPaths = systemFonts

// systemFonts are fonts with wide coverage that operating systems ship, most
// likely first. Only those present are used, and a system with none of them
// draws what the Go font can.
func systemFonts() []string {
	switch runtime.GOOS {
	case "darwin":
		out := []string{
			"/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
			"/Library/Fonts/Arial Unicode.ttf",
			"/System/Library/Fonts/Hiragino Sans GB.ttc",
			"/System/Library/Fonts/AppleSDGothicNeo.ttc",
			"/System/Library/Fonts/Supplemental/KefaIII.ttf", // Ethiopic
			"/System/Library/Fonts/Supplemental/Kailasa.ttc", // Tibetan
		}
		// One Sangam MN font per South and Southeast Asian script: Sinhala,
		// Myanmar, Khmer, Lao and the rest Arial Unicode has or lacks.
		sangam, _ := filepath.Glob("/System/Library/Fonts/Supplemental/*Sangam MN.tt?")
		out = append(out, sangam...)
		// The small Noto fonts macOS ships one script each -- Thaana, Adlam,
		// and ninety more -- for the scripts Arial Unicode lacks. After it,
		// so they are asked only for letters nothing before them has.
		noto, _ := filepath.Glob("/System/Library/Fonts/Supplemental/NotoSans*-Regular.ttf")
		return append(out, noto...)
	case "windows":
		dir := os.Getenv("WINDIR") + `\Fonts\`
		return []string{dir + "arialuni.ttf", dir + "Nirmala.ttf", dir + "LeelawUI.ttf", dir + "msyh.ttc", dir + "malgun.ttf", dir + "seguisym.ttf"}
	}
	var out []string
	for _, dir := range []string{"/usr/share/fonts/truetype/noto/", "/usr/share/fonts/noto/", "/usr/share/fonts/opentype/noto/", "/usr/share/fonts/google-noto/"} {
		for _, f := range []string{"NotoSans-Regular.ttf", "NotoSansThai-Regular.ttf", "NotoSansDevanagari-Regular.ttf",
			"NotoSansBengali-Regular.ttf", "NotoSansTamil-Regular.ttf", "NotoSansArabic-Regular.ttf",
			"NotoSansHebrew-Regular.ttf", "NotoSansCJK-Regular.ttc"} {
			out = append(out, dir+f)
		}
	}
	return append(out, "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf")
}

// fonts are the parsed fonts in the order they are consulted; set by
// loadFonts, and the Go font alone until it is called.
var (
	fontsMu sync.Mutex
	fonts   []*opentype.Font
	faces   = map[int]*render.Fallback{}
)

// loadFonts parses the Go font, then extra, then the system fonts present.
// A --font that cannot be read is an error -- somebody named it -- and a
// system font that cannot be read is passed over.
func loadFonts(extra []string) error {
	goFont, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return fmt.Errorf("parsing the built-in font: %w", err)
	}
	list := []*opentype.Font{goFont}
	for _, path := range extra {
		f, err := parseFont(path)
		if err != nil {
			return fmt.Errorf("--font %s: %w", path, err)
		}
		list = append(list, f)
	}
	for _, path := range systemFontPaths() {
		if f, err := parseFont(path); err == nil {
			list = append(list, f)
		}
	}
	fontsMu.Lock()
	fonts, faces = list, map[int]*render.Fallback{}
	fontsMu.Unlock()
	return nil
}

// parseFont reads a TrueType or OpenType font, or the first font of a
// collection.
func parseFont(path string) (*opentype.Font, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(strings.ToLower(path), ".ttc") || strings.HasPrefix(string(data), "ttcf") {
		c, err := opentype.ParseCollection(data)
		if err != nil {
			return nil, err
		}
		return c.Font(0)
	}
	return opentype.Parse(data)
}

// faceAt is the face at a size in pixels: every loaded font at that size,
// as one. Nil if not even the Go font will load, which draws no text rather
// than no map.
func faceAt(px float64) font.Face {
	key := int(math.Round(max(px, 1)))
	fontsMu.Lock()
	defer fontsMu.Unlock()
	if f, ok := faces[key]; ok {
		return f
	}
	if fonts == nil {
		if f, err := opentype.Parse(goregular.TTF); err == nil {
			fonts = []*opentype.Font{f}
		}
	}
	var fs []font.Face
	for _, ttf := range fonts {
		f, err := opentype.NewFace(ttf, &opentype.FaceOptions{Size: float64(key), DPI: 72, Hinting: font.HintingFull})
		if err == nil {
			fs = append(fs, f)
		}
	}
	fb := render.NewFallback(fs...)
	if fb == nil {
		return nil
	}
	faces[key] = fb
	return fb
}

// missingLetters are the letters asked of any face that no font had.
func missingLetters() []rune {
	fontsMu.Lock()
	defer fontsMu.Unlock()
	seen := map[rune]bool{}
	var out []rune
	for _, f := range faces {
		for _, r := range f.Missing() {
			if !seen[r] {
				seen[r] = true
				out = append(out, r)
			}
		}
	}
	return out
}

// writeMissing tells the user the map has names it could not write, and how
// to get them written.
func writeMissing(w io.Writer) {
	missing := missingLetters()
	if len(missing) == 0 {
		return
	}
	sample := missing
	if len(sample) > 12 {
		sample = sample[:12]
	}
	fmt.Fprintf(w, "course: %d letters in the map's names are in no font this machine has, drawn as boxes: %s\n", len(missing), string(sample))
	fmt.Fprintf(w, "course:   pass --font with a font that has them, or --lang en for the names in English where the map has them\n")
}
