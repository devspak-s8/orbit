//go:build ignore

// Generates the ORBIT desktop icon: a 256x256 dark rounded square with an
// emerald planet and a tilted orbit ring. Run:
//
//	go run main.go
//
// Output: ../assets/icon.png
package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

const size = 256

func main() {
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Rounded dark background.
	bg := color.RGBA{10, 10, 10, 255}
	radius := 52.0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if insideRounded(x, y, radius) {
				img.Set(x, y, bg)
			}
		}
	}

	// Soft radial glow behind the planet.
	cx, cy := 128.0, 138.0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			d := math.Hypot(float64(x)-cx, float64(y)-cy)
			if d < 70 {
				a := uint8(38 * (1 - d/70))
				img.Set(x, y, blend(img.At(x, y), color.RGBA{16, 185, 129, a}))
			}
		}
	}

	// Planet: filled emerald circle with a lighter rim.
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			d := math.Hypot(float64(x)-cx, float64(y)-cy)
			switch {
			case d < 42:
				img.Set(x, y, blend(img.At(x, y), color.RGBA{16, 185, 129, 235}))
			case d < 45:
				img.Set(x, y, blend(img.At(x, y), color.RGBA{52, 211, 153, 255}))
			}
		}
	}

	// Orbit ring: an ellipse rotated -24 degrees. Drawn thick, with the
	// segment behind the planet dimmer for depth.
	angle := -24.0 * math.Pi / 180
	for t := 0.0; t < 360.0; t += 0.15 {
		tr := t * math.Pi / 180
		// Ellipse radii: 96 wide, 34 tall, centered slightly above planet center.
		ex := math.Cos(tr) * 96
		ey := math.Sin(tr) * 34
		px := cx + ex*math.Cos(angle) - ey*math.Sin(angle)
		py := cy - 6 + ex*math.Sin(angle) + ey*math.Cos(angle)

		inFront := math.Sin(tr) >= 0 // lower half of the ring passes in front
		var c color.RGBA
		if inFront {
			c = color.RGBA{52, 211, 153, 255}
		} else {
			c = color.RGBA{22, 101, 52, 255}
		}
		// Stamp a small disk for thickness.
		for dy := -3.0; dy <= 3; dy++ {
			for dx := -3.0; dx <= 3; dx++ {
				if dx*dx+dy*dy <= 9 {
					ix, iy := int(px+dx), int(py+dy)
					if ix >= 0 && iy >= 0 && ix < size && iy < size {
						// Behind the planet: only draw if not covering it.
						pd := math.Hypot(float64(ix)-cx, float64(iy)-cy)
						if !inFront && pd < 42 {
							continue
						}
						img.Set(ix, iy, blend(img.At(ix, iy), c))
					}
				}
			}
		}
	}

	// A small moon on the ring, front-left.
	mx, my := 52.0, 176.0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			d := math.Hypot(float64(x)-mx, float64(y)-my)
			switch {
			case d < 9:
				img.Set(x, y, blend(img.At(x, y), color.RGBA{167, 243, 208, 255}))
			case d < 10.5:
				img.Set(x, y, blend(img.At(x, y), color.RGBA{52, 211, 153, 255}))
			}
		}
	}

	out := filepath.Join("..", "..", "assets", "icon.png")
	os.MkdirAll(filepath.Dir(out), 0o755)
	f, err := os.Create(out)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
	_ = draw.Draw
}

func insideRounded(x, y int, r float64) bool {
	w, h := float64(size), float64(size)
	// Distance to the nearest "inner corner" center.
	nx, ny := r, r
	if float64(x) > w-r {
		nx = w - r
	}
	if float64(y) > h-r {
		ny = h - r
	}
	if float64(x) < r && float64(y) < r {
		nx, ny = r, r
	} else if float64(x) > w-r && float64(y) < r {
		nx, ny = w-r, r
	} else if float64(x) < r && float64(y) > h-r {
		nx, ny = r, h-r
	} else if float64(x) > w-r && float64(y) > h-r {
		nx, ny = w-r, h-r
	} else {
		// Not in a corner quadrant.
		if (float64(x) >= r && float64(x) <= w-r) || (float64(y) >= r && float64(y) <= h-r) {
			return true
		}
	}
	return math.Hypot(float64(x)-nx, float64(y)-ny) <= r
}

func blend(dst color.Color, src color.RGBA) color.RGBA {
	dr, dg, db, _ := dst.RGBA()
	a := float64(src.A) / 255
	return color.RGBA{
		uint8(float64(src.R)*a + float64(dr>>8)*(1-a)),
		uint8(float64(src.G)*a + float64(dg>>8)*(1-a)),
		uint8(float64(src.B)*a + float64(db>>8)*(1-a)),
		255,
	}
}
