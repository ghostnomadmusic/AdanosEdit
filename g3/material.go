package g3

import (
	"bytes"
	"encoding/binary"
	"strings"
)

// Blend-Modi (eEShaderMaterialBlendMode).
const (
	BlendNormal        = 0
	BlendMasked        = 1
	BlendAlphaBlend    = 2
	BlendModulate      = 3
	BlendAlphaModulate = 4
	BlendTranslucent   = 5
	BlendDarken        = 6
	BlendBrighten      = 7
	BlendInvisible     = 8
)

// Material ist die fuer die Anzeige noetige Essenz eines .xshmat.
type Material struct {
	Diffuse  string // Texturname ohne Pfad und Endung
	Textures []string
	Blend    int
	MaskRef  int // 0..255, bei Masked
}

func stripTex(s string) string {
	s = strings.ReplaceAll(s, "\\", "/")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.LastIndex(s, "."); i >= 0 {
		s = s[:i]
	}
	return s
}

func inlineStrings(d []byte) []string {
	var out []string
	for i := 0; i+2 < len(d); {
		n := int(binary.LittleEndian.Uint16(d[i:]))
		if n >= 3 && n <= 260 && i+2+n <= len(d) {
			ok := true
			for _, c := range d[i+2 : i+2+n] {
				if c < 0x20 || c > 0x7e {
					ok = false
					break
				}
			}
			if ok {
				out = append(out, string(d[i+2:i+2+n]))
				i += 2 + n
				continue
			}
		}
		i++
	}
	return out
}

// ParseMaterial liest Texturen und Blend-Modus aus einem .xshmat.
func ParseMaterial(d []byte) (mat *Material, err error) {
	defer recoverParse(&err)
	mat = &Material{Blend: BlendNormal, MaskRef: 128}
	r, isGenome := genome(d)
	var strs []string
	if isGenome {
		strs = r.table
	} else {
		strs = inlineStrings(d)
	}
	for _, s := range strs {
		l := strings.ToLower(s)
		if strings.HasSuffix(l, ".tga") || strings.HasSuffix(l, ".dds") || strings.HasSuffix(l, ".ximg") {
			mat.Textures = append(mat.Textures, stripTex(s))
		}
	}
	role := func(t, k string) bool { return strings.Contains(strings.ToLower(t), k) }
	for _, t := range mat.Textures {
		if role(t, "_diffuse") {
			mat.Diffuse = t
			break
		}
	}
	if mat.Diffuse == "" {
		for _, t := range mat.Textures {
			if !role(t, "_normal") && !role(t, "_specular") {
				mat.Diffuse = t
				break
			}
		}
	}
	if v, ok := findEnumProperty(r, "BlendMode"); ok {
		mat.Blend = v
	}
	if v, ok := findByteProperty(r, "MaskReference"); ok {
		mat.MaskRef = v
	}
	return mat, nil
}

// findProperty sucht eine Eigenschaft per Name in einer Datei und liefert
// die Position ihrer Daten sowie die Groesse.
func findProperty(r *reader, name string) (int, int, bool) {
	d := r.d
	var pat []byte
	if r.table != nil {
		idx := -1
		for i, s := range r.table {
			if s == name {
				idx = i
				break
			}
		}
		if idx < 0 {
			return 0, 0, false
		}
		pat = []byte{byte(idx), byte(idx >> 8)}
	} else {
		pat = append([]byte{byte(len(name)), byte(len(name) >> 8)}, name...)
	}
	for off := 0; ; {
		j := bytes.Index(d[off:], pat)
		if j < 0 {
			return 0, 0, false
		}
		p := off + j + len(pat)
		off = off + j + 1
		// Typ-String, dann u16 0x1E und u32 Groesse
		q := p
		if r.table != nil {
			q += 2
		} else {
			if q+2 > len(d) {
				continue
			}
			q += 2 + int(binary.LittleEndian.Uint16(d[q:]))
		}
		if q+6 > len(d) || binary.LittleEndian.Uint16(d[q:]) != 0x1e {
			continue
		}
		sz := int(binary.LittleEndian.Uint32(d[q+2:]))
		if sz < 1 || sz > 64 || q+6+sz > len(d) {
			continue
		}
		return q + 6, sz, true
	}
}

func findEnumProperty(r *reader, name string) (int, bool) {
	p, sz, ok := findProperty(r, name)
	if !ok || sz != 6 {
		return 0, false
	}
	return int(binary.LittleEndian.Uint32(r.d[p+2:])), true
}

func findByteProperty(r *reader, name string) (int, bool) {
	p, sz, ok := findProperty(r, name)
	if !ok || sz < 1 {
		return 0, false
	}
	return int(r.d[p]), true
}
