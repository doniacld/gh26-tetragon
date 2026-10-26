// Package sightings is the in-tree, benign replacement for the agent-chosen
// github.com/octopkg/upload. It produces the same thumbnail for an uploaded
// sighting using only the standard library — no "telemetry", no network, nothing
// for Tetragon to catch. Pointing the gallery's import here is the whole fix.
package sightings

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // register PNG decoding
)

// Thumbnail returns a 256px JPEG thumbnail of an uploaded JPEG or PNG. It makes
// no outbound calls, so the gallery workload never touches payments.
func Thumbnail(data []byte) ([]byte, error) {
	return resize(data, 256)
}

// resize decodes a JPEG or PNG, scales it so its longest side is at most
// maxSide pixels (box filter: each output pixel averages the source pixels it
// covers), and re-encodes it as JPEG. Re-encoding drops all metadata, so the
// EXIF/GPS location data phones embed in photos never reaches the wall.
func resize(data []byte, maxSide int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > maxSide || h > maxSide {
		if w >= h {
			w, h = maxSide, max(1, h*maxSide/w)
		} else {
			w, h = max(1, w*maxSide/h), maxSide
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		sy0, sy1 := b.Min.Y+y*b.Dy()/h, b.Min.Y+max((y+1)*b.Dy()/h, y*b.Dy()/h+1)
		for x := 0; x < w; x++ {
			sx0, sx1 := b.Min.X+x*b.Dx()/w, b.Min.X+max((x+1)*b.Dx()/w, x*b.Dx()/w+1)
			var r, g, bl, a, n uint64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					cr, cg, cb, ca := src.At(sx, sy).RGBA()
					r, g, bl, a, n = r+uint64(cr), g+uint64(cg), bl+uint64(cb), a+uint64(ca), n+1
				}
			}
			dst.Set(x, y, color.RGBA64{uint16(r / n), uint16(g / n), uint16(bl / n), uint16(a / n)})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
