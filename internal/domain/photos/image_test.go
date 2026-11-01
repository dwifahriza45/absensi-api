package photos

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestNormalizeJPEGCompression(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 960, 720))
	var seed uint32 = 17
	for y := 0; y < 720; y++ {
		for x := 0; x < 960; x++ {
			seed = seed*1664525 + 1013904223
			img.SetRGBA(x, y, color.RGBA{uint8(x / 4), uint8(y / 3), uint8(seed >> 24), 255})
		}
	}
	var input bytes.Buffer
	if err := jpeg.Encode(&input, img, &jpeg.Options{Quality: 82}); err != nil {
		t.Fatal(err)
	}
	payload := []byte("private metadata or appended payload")
	output, mime, ext, err := normalizeImage(append(input.Bytes(), payload...))
	if err != nil || mime != "image/jpeg" || ext != "jpg" {
		t.Fatalf("JPEG normalization: %s %s %v", mime, ext, err)
	}
	if bytes.Contains(output, payload) {
		t.Fatal("appended payload preserved")
	}
	decoded, err := jpeg.Decode(bytes.NewReader(output))
	if err != nil || decoded.Bounds() != img.Bounds() {
		t.Fatalf("compressed image dimensions changed or image invalid: %v", err)
	}
	var previous bytes.Buffer
	decodedInput, err := jpeg.Decode(bytes.NewReader(input.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if err = jpeg.Encode(&previous, decodedInput, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	if len(output) >= previous.Len() {
		t.Fatalf("compression did not reduce fixture: %d >= %d", len(output), previous.Len())
	}
	t.Logf("Synthetic fixture only: old=%d new=%d bytes", previous.Len(), len(output))
}

func TestNormalizeImage(t *testing.T) {
	var input bytes.Buffer
	if err := png.Encode(&input, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	payload := []byte("<script>payload</script>")
	output, contentType, ext, err := normalizeImage(append(input.Bytes(), payload...))
	if err != nil || contentType != "image/png" || ext != "png" {
		t.Fatalf("image: %s %s %v", contentType, ext, err)
	}
	if bytes.Contains(output, payload) {
		t.Fatal("appended payload preserved")
	}
	if _, err = png.Decode(bytes.NewReader(output)); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{[]byte("<svg onload=alert(1)></svg>"), input.Bytes()[:20], []byte("GIF89a")} {
		if _, _, _, err = normalizeImage(bad); err == nil {
			t.Fatal("invalid/unsupported image accepted")
		}
	}
	// Valid IHDR with hostile dimensions must fail before image allocation.
	huge := append([]byte(nil), input.Bytes()...)
	binary.BigEndian.PutUint32(huge[16:20], 100000)
	binary.BigEndian.PutUint32(huge[29:33], crc32.ChecksumIEEE(huge[12:29]))
	if _, _, _, err = normalizeImage(huge); err == nil {
		t.Fatal("pixel bomb accepted")
	}
}
