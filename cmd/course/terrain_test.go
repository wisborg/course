package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wisborg/osmbase/acquire"
	"github.com/wisborg/osmbase/fetch"
	"github.com/wisborg/osmbase/mvt"
	"github.com/wisborg/osmbase/osmbasetest"
	"github.com/wisborg/osmbase/pmtiles"
)

// worldMap fills a store at root with one zoom-0 map tile, all land, from an
// archive written for the purpose: a map to shade, with no network.
func worldMap(t *testing.T, root string) {
	t.Helper()
	tile, err := osmbasetest.BuildTile(osmbasetest.TileSpec{Layers: []osmbasetest.LayerSpec{{
		Name: "earth", Extent: 4096,
		Features: []osmbasetest.FeatureSpec{{
			Type: mvt.GeomPolygon,
			Geometry: mvt.Geometry{Polygons: []mvt.Polygon{{
				Exterior: mvt.Ring{{X: -64, Y: -64}, {X: 4160, Y: -64}, {X: 4160, Y: 4160}, {X: -64, Y: 4160}},
			}}},
		}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "world.pmtiles")
	writeArchive(t, path, tile, pmtiles.TileTypeMVT, `{"attribution":"© test map"}`)
	a, err := fetch.Open(path, fetch.Options{RequireVectorTiles: true})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if _, err := fetch.Fill(context.Background(), root, a, "© test map", acquire.Request{World: true, MaxZoom: 0}, nil, nil); err != nil {
		t.Fatal(err)
	}
}

func writeArchive(t *testing.T, path string, tile []byte, typ pmtiles.TileType, meta string) {
	t.Helper()
	comp := pmtiles.CompressionNone
	built, err := osmbasetest.BuildArchive(osmbasetest.Archive{
		Tiles: []osmbasetest.ArchiveTile{{ID: 0, Data: tile}}, TileType: typ, TileCompression: comp,
		MinZoom: 0, MaxZoom: 12, MinLon: -180, MinLat: -85, MaxLon: 180, MaxLat: 85, Metadata: []byte(meta),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, built.Bytes, 0o644); err != nil {
		t.Fatal(err)
	}
}

// terrainDir is a directory of terrain archives: one zoom-0 Terrarium tile
// of ground rising steeply to the east, so any view of it is a slope.
func terrainDir(t *testing.T) string {
	t.Helper()
	const n = 64
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			h := float64(x)*400 + 32768 + 1000*math.Sin(float64(y)/3)
			img.SetNRGBA(x, y, color.NRGBA{uint8(int(h) >> 8), uint8(int(h)), 0, 0xff})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeArchive(t, filepath.Join(dir, "planet.pmtiles"), b.Bytes(), pmtiles.TileTypePNG, "")
	return dir
}

// --terrain offers to fetch the terrain beside the store, and with --yes
// shades the map from it, says so, and prints the notice the picture's
// credit points to; --contours=false keeps the shading and drops the lines;
// without anybody to say yes, the map is drawn unshaded and the report says
// why.
func TestMapWithTerrain(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }
	resetFlags(t, mapCmd)

	dir := t.TempDir()
	store, out, gpx := filepath.Join(dir, "store"), filepath.Join(dir, "run.png"), filepath.Join(dir, "run.gpx")
	worldMap(t, store)
	writeLine(t, gpx, 10, 20, 0.002, 50)
	tdir := terrainDir(t)

	run := func(args ...string) (string, string) {
		t.Helper()
		resetNow(mapCmd)
		var stdout, stderr bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(&stderr)
		root.SetArgs(append([]string{"map", "--store", store, "--out", out, "--width", "400", "--height", "300", gpx}, args...))
		defer root.SetArgs(nil)
		if err := root.Execute(); err != nil {
			t.Fatalf("map %v: %v\n%s", args, err, stderr.String())
		}
		return stdout.String(), stderr.String()
	}

	stdout, stderr := run("--terrain", "--terrain-source", tdir)
	if !strings.Contains(stderr, "no terrain was fetched") || !strings.Contains(stdout, "the ground is not shaded") {
		t.Errorf("with nobody to answer:\n%s%s", stderr, stdout)
	}
	if _, err := os.Stat(store + "-terrain"); !os.IsNotExist(err) {
		t.Errorf("a terrain store was created with nobody's consent: %v", err)
	}

	stdout, stderr = run("--terrain", "--terrain-source", tdir, "--yes")
	if !strings.Contains(stderr, "copied from "+tdir) || !strings.Contains(stdout, "terrain    100% of the map") ||
		!strings.Contains(stdout, "contours every") || !strings.Contains(stdout, "give this notice with it:\nElevation: ") {
		t.Errorf("with --yes:\n%s%s", stderr, stdout)
	}
	if _, err := os.Stat(store + "-terrain"); err != nil {
		t.Errorf("no terrain store beside the map's: %v", err)
	}

	stdout, _ = run("--set", "map.terrain=true", "--contours=false")
	if !strings.Contains(stdout, "terrain    100% of the map") || strings.Contains(stdout, "contours every") {
		t.Errorf("--set map.terrain=true --contours=false:\n%s", stdout)
	}

	stdout, _ = run()
	if strings.Contains(stdout, "terrain") {
		t.Errorf("terrain without asking for it:\n%s", stdout)
	}
}
