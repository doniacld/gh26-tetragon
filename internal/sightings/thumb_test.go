package sightings

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestThumbnailResizesToJPEG(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 1000, 500))
	for x := 0; x < 1000; x++ {
		for y := 0; y < 500; y++ {
			src.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	var in bytes.Buffer
	if err := png.Encode(&in, src); err != nil {
		t.Fatal(err)
	}
	out, err := Thumbnail(in.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if format != "jpeg" || cfg.Width != 256 || cfg.Height != 128 {
		t.Fatalf("got %s %dx%d, want jpeg 256x128", format, cfg.Width, cfg.Height)
	}
}

func TestThumbnailStripsEXIF(t *testing.T) {
	var plain bytes.Buffer
	if err := jpeg.Encode(&plain, image.NewRGBA(image.Rect(0, 0, 40, 30)), nil); err != nil {
		t.Fatal(err)
	}
	// Insert an APP1 "Exif" segment (where phones store GPS) right after SOI.
	exif := append([]byte{0xFF, 0xE1, 0x00, 0x10}, []byte("Exif\x00\x00GPS-DATA")...)
	withExif := append(append([]byte{}, plain.Bytes()[:2]...), append(exif, plain.Bytes()[2:]...)...)
	out, err := Thumbnail(withExif)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("Exif")) || bytes.Contains(out, []byte("GPS-DATA")) {
		t.Fatal("thumbnail still contains EXIF metadata")
	}
}

func TestThumbnailRejectsNonImages(t *testing.T) {
	if _, err := Thumbnail([]byte("definitely not an image")); err == nil {
		t.Fatal("want an error for non-image input")
	}
}
