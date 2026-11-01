// Generates deterministic synthetic image data for upload benchmarks.
package main

import (
	"flag"
	"image"
	"image/color"
	"image/jpeg"
	"log"
	"math/rand"
	"os"
)

func main() {
	out := flag.String("out", "/tmp/absensi-load.jpg", "output JPEG")
	flag.Parse()
	img := image.NewRGBA(image.Rect(0, 0, 640, 480))
	rng := rand.New(rand.NewSource(42))
	for y := 0; y < 480; y++ {
		for x := 0; x < 640; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256)), 255})
		}
	}
	f, err := os.OpenFile(*out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatal(err)
	}
	if err = jpeg.Encode(f, img, &jpeg.Options{Quality: 85}); err != nil {
		log.Fatal(err)
	}
	if err = f.Close(); err != nil {
		log.Fatal(err)
	}
}
