package g3

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Texturformate
const (
	TexDXT1 = iota + 1
	TexDXT3
	TexDXT5
	TexBGRA8 // unkomprimiert 32 Bit
)

// Image ist eine Textur mit Mipmap-Kette (groesste Stufe zuerst).
type Image struct {
	Width, Height int
	Format        int
	Levels        [][]byte
}

func levelSize(format, w, h int) int {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	switch format {
	case TexDXT1:
		return ((w + 3) / 4) * ((h + 3) / 4) * 8
	case TexDXT3, TexDXT5:
		return ((w + 3) / 4) * ((h + 3) / 4) * 16
	case TexBGRA8:
		return w * h * 4
	}
	return 0
}

// ParseXimg liest ein .ximg. Die Mip-Stufen liegen von klein nach gross
// direkt vor der Stringtabelle.
func ParseXimg(d []byte) (img *Image, err error) {
	defer recoverParse(&err)
	lim := len(d)
	if lim > 512 {
		lim = 512
	}
	g := bytes.Index(d[:lim], []byte("G3IMG"))
	if g < 0 {
		return nil, fmt.Errorf("kein G3IMG-Kopf")
	}
	r := &reader{d: d, p: g + 5}
	r.skip(4)
	w := int(r.u32())
	h := int(r.u32())
	r.skip(8)
	mips := int(r.u32())
	r.u32()
	fourRaw := r.bytes(4)
	four := string(fourRaw)
	d3dfmt := binary.LittleEndian.Uint32(fourRaw)
	if w <= 0 || h <= 0 || w > 16384 || h > 16384 {
		return nil, fmt.Errorf("unplausible Groesse %dx%d", w, h)
	}
	img = &Image{Width: w, Height: h}
	switch four {
	case "DXT1":
		img.Format = TexDXT1
	case "DXT3":
		img.Format = TexDXT3
	case "DXT5":
		img.Format = TexDXT5
	default:
		bpp := rawFormatBytes(d3dfmt)
		if bpp == 0 {
			return nil, fmt.Errorf("Pixelformat %d nicht unterstuetzt", d3dfmt)
		}
		img.Format = TexBGRA8
		return parseRawXimg(d, r.p, img, mips, d3dfmt, bpp)
	}
	end := len(d)
	if string(d[:8]) == "GENOMFLE" {
		end = int(binary.LittleEndian.Uint32(d[10:14]))
		if end > len(d) || end <= 0 {
			end = len(d)
		}
	}
	if mips < 1 {
		mips = 1
	}
	if mips > 16 {
		mips = 16
	}
	// Von der groessten Stufe (am Ende) rueckwaerts lesen.
	pos := end
	lw, lh := w, h
	for i := 0; i < mips; i++ {
		sz := levelSize(img.Format, lw, lh)
		if pos-sz < r.p {
			break
		}
		img.Levels = append(img.Levels, d[pos-sz:pos])
		pos -= sz
		if lw == 1 && lh == 1 {
			break
		}
		lw, lh = max(lw/2, 1), max(lh/2, 1)
	}
	if len(img.Levels) == 0 {
		return nil, fmt.Errorf("Bilddaten unvollstaendig")
	}
	return img, nil
}

// D3DFORMAT-Codes unkomprimierter Formate
const (
	d3dR8G8B8   = 20
	d3dA8R8G8B8 = 21
	d3dX8R8G8B8 = 22
	d3dR5G6B5   = 23
	d3dX1R5G5B5 = 24
	d3dA1R5G5B5 = 25
	d3dA4R4G4B4 = 26
	d3dA8       = 28
	d3dL8       = 50
	d3dA8L8     = 51
)

func rawFormatBytes(f uint32) int {
	switch f {
	case d3dA8R8G8B8, d3dX8R8G8B8:
		return 4
	case d3dR8G8B8:
		return 3
	case d3dR5G6B5, d3dX1R5G5B5, d3dA1R5G5B5, d3dA4R4G4B4, d3dA8L8:
		return 2
	case d3dA8, d3dL8:
		return 1
	}
	return 0
}

func ext5(v uint16) byte { return byte(v<<3 | v>>2) }
func ext6(v uint16) byte { return byte(v<<2 | v>>4) }
func ext4(v uint16) byte { return byte(v<<4 | v) }

// toBGRA wandelt eine Stufe eines unkomprimierten Formats nach BGRA8.
func toBGRA(src []byte, n int, f uint32) []byte {
	out := make([]byte, n*4)
	for i := 0; i < n; i++ {
		o := out[i*4 : i*4+4]
		switch f {
		case d3dA8R8G8B8:
			copy(o, src[i*4:i*4+4])
		case d3dX8R8G8B8:
			copy(o, src[i*4:i*4+3])
			o[3] = 255
		case d3dR8G8B8:
			copy(o, src[i*3:i*3+3])
			o[3] = 255
		case d3dR5G6B5:
			v := binary.LittleEndian.Uint16(src[i*2:])
			o[0], o[1], o[2], o[3] = ext5(v&31), ext6(v>>5&63), ext5(v>>11&31), 255
		case d3dX1R5G5B5, d3dA1R5G5B5:
			v := binary.LittleEndian.Uint16(src[i*2:])
			o[0], o[1], o[2] = ext5(v&31), ext5(v>>5&31), ext5(v>>10&31)
			o[3] = 255
			if f == d3dA1R5G5B5 && v>>15 == 0 {
				o[3] = 0
			}
		case d3dA4R4G4B4:
			v := binary.LittleEndian.Uint16(src[i*2:])
			o[0], o[1], o[2], o[3] = ext4(v&15), ext4(v>>4&15), ext4(v>>8&15), ext4(v>>12&15)
		case d3dA8L8:
			o[0], o[1], o[2], o[3] = src[i*2], src[i*2], src[i*2], src[i*2+1]
		case d3dA8:
			o[0], o[1], o[2], o[3] = 255, 255, 255, src[i]
		case d3dL8:
			o[0], o[1], o[2], o[3] = src[i], src[i], src[i], 255
		}
	}
	return out
}

func parseRawXimg(d []byte, minPos int, img *Image, mips int, f uint32, bpp int) (*Image, error) {
	end := len(d)
	if string(d[:8]) == "GENOMFLE" {
		end = int(binary.LittleEndian.Uint32(d[10:14]))
		if end > len(d) || end <= 0 {
			end = len(d)
		}
	}
	if mips < 1 {
		mips = 1
	}
	pos := end
	lw, lh := img.Width, img.Height
	for i := 0; i < mips && i < 16; i++ {
		n := lw * lh
		sz := n * bpp
		if pos-sz < minPos {
			break
		}
		img.Levels = append(img.Levels, toBGRA(d[pos-sz:pos], n, f))
		pos -= sz
		if lw == 1 && lh == 1 {
			break
		}
		lw, lh = max(lw/2, 1), max(lh/2, 1)
	}
	if len(img.Levels) == 0 {
		return nil, fmt.Errorf("Bilddaten unvollstaendig")
	}
	return img, nil
}
