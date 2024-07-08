/*
    Source: https://github.com/gerow/go-color/blob/master/color.go
	Package color implements some simple RGB/HSL color conversions for golang.
	By Brandon Thomson
	Adapted from
	http://code.google.com/p/closure-library/source/browse/trunk/closure/goog/color/color.js
	and algorithms on easyrgb.com.
	To maintain accuracy between conversions we use floats in the color types.
	If you are storing lots of colors and care about memory use you might want
	to use something based on byte types instead.
	Also, color types don't verify their validity before converting. If you do
	something like RGB{10,20,30}.ToHSL() the results will be undefined. All
	values must be between 0 and 1.
*/

package base

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
)

type Color3b struct {
	R, G, B uint8
}
type Color3f struct {
	R, G, B float64
}

func NewColor3b(r, g, b uint8) Color3b   { return Color3b{R: r, G: g, B: b} }
func NewColor3f(r, g, b float64) Color3f { return Color3f{R: r, G: g, B: b} }

func (x *Color3b) Broadcast(v uint8) {
	x.R = v
	x.G = v
	x.B = v
}
func (x *Color3f) Broadcast(v float64) {
	x.R = v
	x.G = v
	x.B = v
}

// RGBA returns the alpha-premultiplied red, green, blue and alpha values
// for the color. Each value ranges within [0, 0xffff], but is represented
// by a uint32 so that multiplying by a blend factor up to 0xffff will not
// overflow.
//
// An alpha-premultiplied color component c has been scaled by alpha (a),
// so has valid values 0 <= c <= a.
func (x Color3b) RGBA() (r, g, b, a uint32) {
	r = uint32(x.R) * 257
	g = uint32(x.G) * 257
	b = uint32(x.B) * 257
	a = 0xFFFF
	return
}
func (x Color3f) RGBA() (r, g, b, a uint32) {
	r = uint32(x.R * 0xFFFF)
	g = uint32(x.G * 0xFFFF)
	b = uint32(x.B * 0xFFFF)
	a = 0xFFFF
	return
}

const maxUint8f = float64(math.MaxUint8)
const maxUint8OO = 1.0 / maxUint8f

func unquantizeColor(b uint8) float64 { return float64(b) * maxUint8OO }
func quantizeColor(f float64) uint8   { return uint8(f * maxUint8f) }

func (x Color3b) Unquantize(srgb bool) (result Color3f) {
	result = Color3f{
		R: unquantizeColor(x.R),
		G: unquantizeColor(x.G),
		B: unquantizeColor(x.B),
	}
	if srgb {
		result = result.SrgbToLinear()
	}
	return
}
func (x Color3f) Quantize(srgb bool) (result Color3b) {
	if srgb {
		x = x.LinearToSrgb()
	}
	result = Color3b{
		R: quantizeColor(x.R),
		G: quantizeColor(x.G),
		B: quantizeColor(x.B),
	}
	return
}

func (x Color3b) Ansi(fg bool) string {
	if !enableAnsiColor {
		return ""
	}
	ansiFmt := ANSI_BG_TRUECOLOR_FMT
	if fg {
		ansiFmt = ANSI_FG_TRUECOLOR_FMT
	}
	return fmt.Sprintf(ansiFmt.String(), uint(x.R), uint(x.G), uint(x.B))
}
func (x Color3b) Lerp(o Color3b, f float64) Color3b {
	return x.Unquantize(false).Lerp(o.Unquantize(false), f).Quantize(false)
}

var colorHslHot = Color3b{R: 255, G: 210, B: 128}.Unquantize(true)
var colorHslCold = Color3b{R: 103, G: 79, B: 73}.Unquantize(true)

func NewColdHotColor(f float64) Color3f {
	color := colorHslCold.Lerp(colorHslHot, f)
	return color.Brightness(0.45 + 0.40*f*f)
}

func NewHeatmapColor(f float64) Color3f {
	f = Saturate(f)
	x1 := [4]float64{1, f, f * f, f * f * f}   // 1 x x2 x3
	x2 := [2]float64{x1[3] * f, x1[3] * f * f} // x4 x5
	return Color3f{
		R: Saturate(Dot(x1[:], []float64{-0.027780558, +1.228188385, +0.278906882, +3.892783760}) + Dot(x2[:], []float64{-8.490712758, +4.069046086})),
		G: Saturate(Dot(x1[:], []float64{+0.014065206, +0.015360518, +1.605395918, -4.821108251}) + Dot(x2[:], []float64{+8.389314011, -4.193858954})),
		B: Saturate(Dot(x1[:], []float64{-0.019628385, +3.122510347, -5.893222355, +2.798380308}) + Dot(x2[:], []float64{-3.608884658, +4.324996022})),
	}
}
func NewPastelizerColor(f float64) Color3f {
	_, h := math.Modf(Saturate(f) + 0.92620819117478)
	h = math.Abs(h) * 6.2831853071796
	cocg_x, cocg_y := 0.25*math.Cos(h), 0.25*math.Sin(h)
	br_x, br_y := -cocg_x-cocg_y, cocg_x-cocg_y
	c_x, c_y, c_z := 0.729+br_y, 0.729+cocg_y, 0.729+br_x
	return Color3f{
		R: Saturate(c_x * c_x),
		G: Saturate(c_y * c_y),
		B: Saturate(c_z * c_z),
	}
}

func nextFloat01(r *rand.Rand) float64 {
	// see official comment in func (r *Rand) Float64() float64
	return float64(r.Int63n(1<<53)) / (1 << 53)
}

func NewColorFromHash(h uint64) Color3f {
	rnd := rand.New(rand.NewSource(int64(h)))
	return NewPastelizerColor(nextFloat01(rnd))
}
func NewColorFromStringHash(s string) Color3f {
	hasher := fnv.New64a()
	hasher.Write(UnsafeBytesFromString(s))
	return NewColorFromHash(hasher.Sum64())
}

func (x Color3f) Brightness(f float64) Color3f {
	brightness := math.Exp2((f*2.0 - 1.0) * 4.0)
	return Color3f{
		R: Saturate(x.R * brightness),
		G: Saturate(x.G * brightness),
		B: Saturate(x.B * brightness),
	}
}
func (x Color3f) Lerp(o Color3f, f float64) Color3f {
	return Color3f{
		R: Lerp(x.R, o.R, f),
		G: Lerp(x.G, o.G, f),
		B: Lerp(x.B, o.B, f),
	}
}
func (x Color3f) SrgbToLinear() Color3f {
	return Color3f{
		R: linearizeColor(x.R),
		G: linearizeColor(x.G),
		B: linearizeColor(x.B),
	}
}
func (x Color3f) LinearToSrgb() Color3f {
	return Color3f{
		R: delinearizeColor(x.R),
		G: delinearizeColor(x.G),
		B: delinearizeColor(x.B),
	}
}

func linearizeColor(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	} else {
		return math.Pow(float64((c+0.055)/1.055), 2.4)
	}
}
func delinearizeColor(c float64) float64 {
	if c <= 0.0031308 {
		return 12.92 * c
	} else {
		return 1.055*math.Pow(float64(c), 1/2.4) - 0.055
	}
}

// A nudge to make truncation round to nearest number instead of flooring

func (x Color3b) ToHTML(alpha uint8) string {
	return fmt.Sprintf("#%02x%02x%02x%02x", x.R, x.G, x.B, alpha)
}
