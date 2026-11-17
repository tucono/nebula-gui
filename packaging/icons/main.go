// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

// Command icons renders the app icon's SVG source to the files the
// packages need: PNGs for Linux's hicolor theme and fyne package, an ICO
// for Windows and an ICNS for macOS. Run it with make icons.
//
//	go run ./packaging/icons [-src internal/gui/assets] [-out dist/icons]
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"github.com/fyne-io/oksvg"
	"github.com/srwiley/rasterx"
)

// pngSizes are the app icon's PNG sizes: the hicolor theme's, and 1024
// for fyne package.
var pngSizes = []int{16, 24, 32, 48, 64, 128, 256, 512, 1024}

// icoSizes go in the Windows icon.
var icoSizes = []int{16, 24, 32, 48, 64, 256}

// icnsTypes are the ICNS entries, all PNG, by pixel size.
var icnsTypes = []struct {
	typ  string
	size int
}{
	{"icp4", 16}, {"icp5", 32}, {"icp6", 64}, {"ic07", 128}, {"ic08", 256}, {"ic09", 512},
	{"ic10", 1024}, {"ic11", 32}, {"ic12", 64}, {"ic13", 256}, {"ic14", 512},
}

func main() {
	src := flag.String("src", "internal/gui/assets", "directory with the SVG sources")
	out := flag.String("out", "dist/icons", "output directory")
	flag.Parse()
	if err := run(*src, *out); err != nil {
		fmt.Fprintln(os.Stderr, "icons:", err)
		os.Exit(1)
	}
}

func run(src, out string) error {
	svg, err := os.ReadFile(filepath.Join(src, "app.svg")) //nolint:gosec // a path the developer gives
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil { //nolint:gosec // build output, packaged for everyone
		return err
	}
	pngs := map[int][]byte{}
	for _, n := range append(append([]int{}, pngSizes...), icoSizes...) {
		if pngs[n] != nil {
			continue
		}
		b, err := render(svg, n)
		if err != nil {
			return err
		}
		pngs[n] = b
	}
	for _, n := range pngSizes {
		if err := os.WriteFile(filepath.Join(out, fmt.Sprintf("nebula-gui-%d.png", n)), pngs[n], 0o644); err != nil { //nolint:gosec // build output, packaged for everyone
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(out, "nebula-gui.ico"), ico(pngs), 0o644); err != nil { //nolint:gosec // build output, packaged for everyone
		return err
	}
	return os.WriteFile(filepath.Join(out, "nebula-gui.icns"), icns(pngs), 0o644) //nolint:gosec // build output, packaged for everyone
}

// render draws the SVG at n×n pixels and encodes it as PNG.
func render(svg []byte, n int) ([]byte, error) {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(svg))
	if err != nil {
		return nil, err
	}
	icon.SetTarget(0, 0, float64(n), float64(n))
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	icon.Draw(rasterx.NewDasher(n, n, rasterx.NewScannerGV(n, n, img, img.Bounds())), 1)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ico writes an ICO file whose entries are PNGs, which Windows Vista and
// later read.
func ico(pngs map[int][]byte) []byte {
	var buf bytes.Buffer
	le := binary.LittleEndian
	_ = binary.Write(&buf, le, [3]uint16{0, 1, uint16(len(icoSizes))}) //nolint:gosec // six sizes
	offset := 6 + 16*len(icoSizes)
	for _, n := range icoSizes {
		var dim byte // 256 is written as 0
		if n < 256 {
			dim = byte(n) //nolint:gosec // checked above
		}
		buf.Write([]byte{dim, dim, 0, 0})
		_ = binary.Write(&buf, le, [2]uint16{1, 32})
		_ = binary.Write(&buf, le, [2]uint32{u32(len(pngs[n])), u32(offset)})
		offset += len(pngs[n])
	}
	for _, n := range icoSizes {
		buf.Write(pngs[n])
	}
	return buf.Bytes()
}

// icns writes an ICNS file whose entries are PNGs.
func icns(pngs map[int][]byte) []byte {
	var body bytes.Buffer
	for _, t := range icnsTypes {
		body.WriteString(t.typ)
		_ = binary.Write(&body, binary.BigEndian, u32(8+len(pngs[t.size])))
		body.Write(pngs[t.size])
	}
	var buf bytes.Buffer
	buf.WriteString("icns")
	_ = binary.Write(&buf, binary.BigEndian, u32(8+body.Len()))
	buf.Write(body.Bytes())
	return buf.Bytes()
}

// u32 is n as a uint32. The files are well under a megabyte.
func u32(n int) uint32 {
	return uint32(n) //nolint:gosec // see above
}
