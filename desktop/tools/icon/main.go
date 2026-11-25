// icon renders build/appicon.png (1024×1024) from the ReproPack mark using
// supersampled signed-distance shapes. Run: go run ./tools/icon
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
)

func sdRoundRect(px, py, cx, cy, hw, hh, r float64) float64 {
	qx := math.Abs(px-cx) - hw + r
	qy := math.Abs(py-cy) - hh + r
	return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - r
}

func sdSeg(px, py, ax, ay, bx, by float64) float64 {
	pax, pay, bax, bay := px-ax, py-ay, bx-ax, by-ay
	h := math.Max(0, math.Min(1, (pax*bax+pay*bay)/(bax*bax+bay*bay)))
	return math.Hypot(pax-bax*h, pay-bay*h)
}

func main() {
	const N, S = 1024, 4
	img := image.NewNRGBA(image.Rect(0, 0, N, N))
	bgTop := [3]float64{24, 27, 31}
	bgBot := [3]float64{11, 12, 15}
	seal := [3]float64{60, 203, 170}
	white := [3]float64{240, 242, 238}
	for y := 0; y < N; y++ {
		for x := 0; x < N; x++ {
			var acc [4]float64
			for sy := 0; sy < S; sy++ {
				for sx := 0; sx < S; sx++ {
					px := (float64(x) + (float64(sx)+0.5)/S) / N * 20
					py := (float64(y) + (float64(sy)+0.5)/S) / N * 20
					// macOS squircle-ish tile with margin
					if sdRoundRect(px, py, 10, 10, 8.1, 8.1, 1.9) > 0 {
						continue
					}
					t := py / 20
					c := [3]float64{bgTop[0]*(1-t) + bgBot[0]*t, bgTop[1]*(1-t) + bgBot[1]*t, bgTop[2]*(1-t) + bgBot[2]*t}
					// pack body outline (stroke)
					body := math.Abs(sdRoundRect(px, py, 10, 11.2, 4.4, 3.5, 1.0)) - 0.42
					handle := math.Min(sdSeg(px, py, 7.6, 7.7, 7.6, 6.6), math.Min(sdSeg(px, py, 12.4, 7.7, 12.4, 6.6), sdSeg(px, py, 7.6, 6.6, 12.4, 6.6))) - 0.42
					if body < 0 || handle < 0 {
						c = white
					}
					// seal line (teal) and short ink line
					if sdSeg(px, py, 7.9, 10.6, 12.1, 10.6)-0.45 < 0 {
						c = seal
					}
					if sdSeg(px, py, 7.9, 12.8, 10.2, 12.8)-0.45 < 0 {
						c = white
					}
					acc[0] += c[0]
					acc[1] += c[1]
					acc[2] += c[2]
					acc[3] += 255
				}
			}
			if acc[3] == 0 {
				continue
			}
			a := acc[3] / float64(S*S) // coverage-weighted alpha
			cov := acc[3] / 255          // number of covered samples
			img.SetNRGBA(x, y, color.NRGBA{uint8(acc[0] / cov), uint8(acc[1] / cov), uint8(acc[2] / cov), uint8(a)})
		}
	}
	f, err := os.Create("build/appicon.png")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, img)
}
