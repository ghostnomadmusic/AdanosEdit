package g3

import "encoding/binary"

func rgb565(c uint16) [3]int {
	return [3]int{int(c>>11&31) * 255 / 31, int(c>>5&63) * 255 / 63, int(c&31) * 255 / 31}
}

// DecodeRGBA dekodiert die groesste Mip-Stufe nach RGBA8.
func (img *Image) DecodeRGBA() []byte {
	w, h := img.Width, img.Height
	out := make([]byte, w*h*4)
	src := img.Levels[0]
	if img.Format == TexBGRA8 {
		for i := 0; i < w*h && i*4+3 < len(src); i++ {
			out[i*4], out[i*4+1], out[i*4+2], out[i*4+3] = src[i*4+2], src[i*4+1], src[i*4], src[i*4+3]
		}
		return out
	}
	bs := 8
	if img.Format != TexDXT1 {
		bs = 16
	}
	bw := (w + 3) / 4
	for by := 0; by < (h+3)/4; by++ {
		for bx := 0; bx < bw; bx++ {
			off := (by*bw + bx) * bs
			if off+bs > len(src) {
				return out
			}
			blk := src[off : off+bs]
			var alpha [16]int
			for i := range alpha {
				alpha[i] = 255
			}
			cb := blk
			switch img.Format {
			case TexDXT3:
				for i := 0; i < 16; i++ {
					a := int(blk[i/2]>>(uint(i%2)*4)) & 15
					alpha[i] = a * 17
				}
				cb = blk[8:]
			case TexDXT5:
				a0, a1 := int(blk[0]), int(blk[1])
				var pal [8]int
				pal[0], pal[1] = a0, a1
				if a0 > a1 {
					for i := 1; i < 7; i++ {
						pal[i+1] = ((7-i)*a0 + i*a1) / 7
					}
				} else {
					for i := 1; i < 5; i++ {
						pal[i+1] = ((5-i)*a0 + i*a1) / 5
					}
					pal[6], pal[7] = 0, 255
				}
				bits := uint64(0)
				for i := 0; i < 6; i++ {
					bits |= uint64(blk[2+i]) << (8 * uint(i))
				}
				for i := 0; i < 16; i++ {
					alpha[i] = pal[bits>>(3*uint(i))&7]
				}
				cb = blk[8:]
			}
			c0 := binary.LittleEndian.Uint16(cb)
			c1 := binary.LittleEndian.Uint16(cb[2:])
			p0, p1 := rgb565(c0), rgb565(c1)
			var pal [4][4]int
			pal[0] = [4]int{p0[0], p0[1], p0[2], 255}
			pal[1] = [4]int{p1[0], p1[1], p1[2], 255}
			if c0 > c1 || img.Format != TexDXT1 {
				for k := 0; k < 3; k++ {
					pal[2][k] = (2*p0[k] + p1[k]) / 3
					pal[3][k] = (p0[k] + 2*p1[k]) / 3
				}
				pal[2][3], pal[3][3] = 255, 255
			} else {
				for k := 0; k < 3; k++ {
					pal[2][k] = (p0[k] + p1[k]) / 2
				}
				pal[2][3] = 255
				pal[3] = [4]int{0, 0, 0, 0}
			}
			idx := binary.LittleEndian.Uint32(cb[4:])
			for i := 0; i < 16; i++ {
				x, y := bx*4+i%4, by*4+i/4
				if x >= w || y >= h {
					continue
				}
				c := pal[idx>>(2*uint(i))&3]
				o := (y*w + x) * 4
				out[o], out[o+1], out[o+2] = byte(c[0]), byte(c[1]), byte(c[2])
				a := c[3]
				if img.Format != TexDXT1 {
					a = alpha[i]
				}
				out[o+3] = byte(a)
			}
		}
	}
	return out
}
