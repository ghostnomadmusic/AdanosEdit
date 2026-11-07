package g3

import (
	"encoding/binary"
	"fmt"
	"math"
)

// SubMesh ist ein Teil eines Meshes mit einem Material.
type SubMesh struct {
	Material string
	Pos      []float32 // xyz
	Normal   []float32 // xyz (kann leer sein)
	UV       []float32 // uv  (kann leer sein)
	Color    []uint32  // Vertexfarben (kann leer sein)
	Index    []uint32
}

// Mesh ist ein geladenes .xcmsh.
type Mesh struct {
	Subs     []SubMesh
	Min, Max [3]float32
}

const (
	streamFace     = 0
	streamPosition = 1
	streamNormal   = 3
	streamDiffuse  = 4
	streamUV       = 12
)

// Elementgroesse je Stream-Typ (aus eEVertexStreamArrayType -> eEVertexTypeStruct).
func streamElemSize(t uint32) int {
	switch {
	case t == 0, t == 4, t == 5:
		return 4 // GEU32
	case t == 1, t == 3:
		return 12
	case t == 2:
		return 16
	case t >= 6 && t <= 11:
		return 4 // GEFloat
	case t == 12:
		return 8
	case t >= 13 && t <= 59:
		switch (t - 13) % 3 {
		case 0:
			return 12
		case 1:
			return 16
		default:
			return 8
		}
	case t >= 60 && t <= 63:
		return 16
	case t >= 64 && t <= 67:
		return 12
	case t >= 68 && t <= 71:
		return 8
	case t == 72:
		return 12
	case t == 73:
		return 8
	}
	return -1
}

func toFloats(b []byte) []float32 {
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out
}

func toU32(b []byte) []uint32 {
	out := make([]uint32, len(b)/4)
	for i := range out {
		out[i] = binary.LittleEndian.Uint32(b[i*4:])
	}
	return out
}

// openResource liest den Kopf einer Ressourcen-Datei (eCResource*_PS) und
// positioniert den Reader hinter den Basis-Ressourcendaten.
func openResource(d []byte) (*reader, string) {
	r, _ := genome(d)
	r.skip(6)
	name := r.entry()
	r.skip(5)
	var ps PropSet
	r.propsBody(&ps)
	r.u16() // Klassenversion
	rv := r.u16()
	if rv >= 0x17 {
		r.u32()
	}
	if rv < 0x1e {
		if r.u16() > 1 {
			r.skip(1)
		}
	}
	return r, name
}

// ParseMesh liest ein .xcmsh (eCResourceMeshComplex_PS).
func ParseMesh(d []byte) (m *Mesh, err error) {
	defer recoverParse(&err)
	if len(d) == 0 {
		return nil, fmt.Errorf("leere Mesh-Datei")
	}
	r, cname := openResource(d)
	if cname != "eCResourceMeshComplex_PS" {
		return nil, fmt.Errorf("kein Mesh (%s)", cname)
	}
	r.f32()
	cnt := int(r.u32())
	if cnt > 10000 {
		fail("unplausible Anzahl Teilmeshes %d", cnt)
	}
	m = &Mesh{Min: [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32},
		Max: [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}}
	for i := 0; i < cnt; i++ {
		ev := r.u16()
		r.u32() // FVF
		r.skip(24)
		r.u32()
		var sm SubMesh
		sm.Material = r.entry()
		sc := int(r.u32())
		for j := 0; j < sc; j++ {
			t := r.u32()
			r.skip(3)
			c := int(r.u32())
			es := streamElemSize(t)
			if es < 0 {
				fail("unbekannter Vertex-Stream %d", t)
			}
			raw := r.bytes(c * es)
			switch t {
			case streamFace:
				sm.Index = toU32(raw)
			case streamPosition:
				sm.Pos = toFloats(raw)
			case streamNormal:
				sm.Normal = toFloats(raw)
			case streamUV:
				sm.UV = toFloats(raw)
			case streamDiffuse:
				sm.Color = toU32(raw)
			}
		}
		if ev >= 3 {
			r.skip(1)
			r.skip(int(r.u32()) * 4)
			r.skip(1)
			r.skip(int(r.u32()) * 4)
		}
		if ev >= 2 {
			r.skip(1)
			g := int(r.u32())
			for k := 0; k < g; k++ {
				r.skip(1)
				r.skip(int(r.u32()) * 4)
				r.skip(1)
				r.skip(int(r.u32()) * 4)
				r.skip(12 + 64 + 8)
			}
		}
		if ev >= 4 {
			r.skip(int(r.u32()) * 24)
			r.skip(1)
			r.skip(int(r.u32()) * 4)
		}
		nv := len(sm.Pos) / 3
		if nv == 0 || len(sm.Index) < 3 {
			continue
		}
		// Ungueltige Indizes und unpassende Streams verwerfen.
		ok := true
		for _, ix := range sm.Index {
			if int(ix) >= nv {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		sm.Index = sm.Index[:len(sm.Index)/3*3]
		if len(sm.Normal) != nv*3 {
			sm.Normal = nil
		}
		if len(sm.UV) != nv*2 {
			sm.UV = nil
		}
		if len(sm.Color) != nv {
			sm.Color = nil
		}
		for v := 0; v < nv; v++ {
			for a := 0; a < 3; a++ {
				x := sm.Pos[v*3+a]
				if x < m.Min[a] {
					m.Min[a] = x
				}
				if x > m.Max[a] {
					m.Max[a] = x
				}
			}
		}
		m.Subs = append(m.Subs, sm)
	}
	if len(m.Subs) == 0 {
		m.Min, m.Max = [3]float32{}, [3]float32{}
	}
	return m, nil
}

// ParseLodMesh liest ein .xlmsh (eCResourceMeshLoD_PS) und liefert die
// enthaltenen Mesh-Namen (hoechste Detailstufe zuerst).
func ParseLodMesh(d []byte) (names []string, err error) {
	defer recoverParse(&err)
	r, cname := openResource(d)
	if cname != "eCResourceMeshLoD_PS" {
		return nil, fmt.Errorf("kein LoD-Mesh (%s)", cname)
	}
	n := int(r.u32())
	for i := 0; i < n && i < 64; i++ {
		names = append(names, r.entry())
	}
	return names, nil
}
