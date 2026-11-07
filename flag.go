//go:build windows

package main

// Kleine Landesflagge neben "Sprache:" (wird zur Laufzeit gezeichnet, ohne
// Bilddateien): Deutschland, Vereinigtes Koenigreich, Russland, Polen.

import (
	"math"
	"unsafe"

	"g3viewer/lang"
)

var (
	pCreateBitmap = gdi32.NewProc("CreateBitmap")
	pDeleteObject = gdi32.NewProc("DeleteObject")
)

const (
	ssBitmap    = 0x000E
	stmSetImage = 0x0172
)

// flagPixels zeichnet die Flagge der Sprache code (w x h, BGRA).
func flagPixels(code string, w, h int) []byte {
	px := make([]byte, w*h*4)
	set := func(x, y int, c uint32) {
		o := (y*w + x) * 4
		px[o], px[o+1], px[o+2], px[o+3] = byte(c), byte(c>>8), byte(c>>16), 255
	}
	diag := math.Hypot(float64(w), float64(h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			var c uint32
			switch code {
			case "de":
				c = [3]uint32{0x000000, 0xDD0000, 0xFFCE00}[y*3/h]
			case "ru":
				c = [3]uint32{0xFFFFFF, 0x0039A6, 0xD52B1E}[y*3/h]
			case "pl":
				c = 0xFFFFFF
				if y*2 >= h {
					c = 0xDC143C
				}
			default: // en: Union Jack (vereinfacht)
				c = 0x012169
				d1 := math.Abs(float64(h)*fx-float64(w)*fy) / diag
				d2 := math.Abs(float64(h)*(float64(w)-fx)-float64(w)*fy) / diag
				t := float64(h)
				if d1 < t*0.12 || d2 < t*0.12 {
					c = 0xFFFFFF
				}
				if d1 < t*0.045 || d2 < t*0.045 {
					c = 0xC8102E
				}
				cx, cy := math.Abs(fx-float64(w)/2), math.Abs(fy-float64(h)/2)
				if cx < t*0.18 || cy < t*0.18 {
					c = 0xFFFFFF
				}
				if cx < t*0.10 || cy < t*0.10 {
					c = 0xC8102E
				}
			}
			if x == 0 || y == 0 || x == w-1 || y == h-1 {
				c = 0x808080 // feiner Rahmen (auch fuer weisse Streifen)
			}
			set(x, y, c)
		}
	}
	return px
}

// updateFlag zeigt die Flagge der eingestellten Sprache.
func (a *app) updateFlag() {
	if a.hFlag == 0 {
		return
	}
	w, h := a.scale(24), a.scale(16)
	px := flagPixels(lang.Code(), w, h)
	bmp, _, _ := pCreateBitmap.Call(uintptr(w), uintptr(h), 1, 32, uintptr(unsafe.Pointer(&px[0])))
	if bmp == 0 {
		return
	}
	old := sendMessage(a.hFlag, stmSetImage, 0, bmp)
	if old != 0 {
		pDeleteObject.Call(old)
	}
	if a.flagBmp != 0 && a.flagBmp != old {
		pDeleteObject.Call(a.flagBmp)
	}
	a.flagBmp = bmp
}

// layoutLang ordnet "Sprache:", Flagge und Auswahl in einer Zeile an.
func (a *app) layoutLang(x, y, w, lh int) {
	lw := a.scale(70)
	moveWindow(a.hLangLbl, x, y+a.scale(5), lw, lh)
	fx := x + lw + a.scale(2)
	moveWindow(a.hFlag, fx, y+a.scale(5), a.scale(24), a.scale(16))
	cx := fx + a.scale(30)
	moveWindow(a.hLang, cx, y, x+w-cx, a.scale(200))
}
