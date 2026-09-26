package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/gomono"
)

func noSystemFonts(t *testing.T) {
	t.Helper()
	saved := systemFontPaths
	systemFontPaths = func() []string { return nil }
	t.Cleanup(func() { systemFontPaths = saved; loadFonts(nil) })
}

// A letter no font has is reported, with what to do about it; one the
// fonts have is not.
func TestMissingLettersAreReported(t *testing.T) {
	noSystemFonts(t)
	if err := loadFonts(nil); err != nil {
		t.Fatal(err)
	}
	f := faceAt(13)
	for _, r := range "Sydneyกรุงเทพ" {
		f.GlyphAdvance(r)
	}
	var w bytes.Buffer
	writeMissing(&w)
	if !strings.Contains(w.String(), "7 letters") || !strings.Contains(w.String(), "ก") ||
		!strings.Contains(w.String(), "--font") || !strings.Contains(w.String(), "--lang en") {
		t.Errorf("the note is:\n%s", w.String())
	}

	if err := loadFonts(nil); err != nil {
		t.Fatal(err)
	}
	faceAt(13).GlyphAdvance('S')
	w.Reset()
	if writeMissing(&w); w.Len() != 0 {
		t.Errorf("a map with every letter written says:\n%s", w.String())
	}
}

// A --font is used after the built-in font, and one that cannot be read is
// an error naming it: somebody asked for it.
func TestFontsGivenAreLoaded(t *testing.T) {
	noSystemFonts(t)
	dir := t.TempDir()
	good := filepath.Join(dir, "mono.ttf")
	if err := os.WriteFile(good, gomono.TTF, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := loadFonts([]string{good}); err != nil {
		t.Fatalf("a real font: %v", err)
	}
	if len(fonts) != 2 {
		t.Errorf("%d fonts loaded, want the Go font and the one given", len(fonts))
	}
	bad := filepath.Join(dir, "not-a-font.ttf")
	os.WriteFile(bad, []byte("hello"), 0o644)
	err := loadFonts([]string{bad})
	if err == nil || !strings.Contains(err.Error(), "not-a-font.ttf") {
		t.Errorf("a file that is not a font: %v", err)
	}
}
