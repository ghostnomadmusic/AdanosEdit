package g3

import (
	"fmt"
	"math"
)

// ParseActor liest ein .xact (eCResourceAnimationActor_PS, EMotionFX-Actor)
// und liefert die Geometrie in der Ruhepose als Mesh im Gothic-Raum.
// lod = 0 waehlt das Hauptmodell.
func ParseActor(d []byte) (m *Mesh, err error) {
	defer recoverParse(&err)
	r, ok := genome(d)
	if !ok {
		return nil, fmt.Errorf("kein Genome-Actor")
	}
	if v := r.u16(); v != 54 {
		return nil, fmt.Errorf("Actor-Version %d nicht unterstuetzt", v)
	}
	r.u32()
	r.f32()
	r.skip(8) // bCDateTime
	r.u32()
	r.skip(24) // Boundary
	n := int(r.u32())
	for i := 0; i < n; i++ { // LookAt-Constraints
		r.entry()
		r.skip(4 + 12 + 12)
	}
	lods := int(r.u32())
	if lods > 32 {
		fail("unplausible LoD-Anzahl %d", lods)
	}
	for i := 0; i < lods; i++ {
		r.readActorWrapper(false)
	}
	return r.readActorWrapper(true), nil
}

func (r *reader) readActorWrapper(keep bool) *Mesh {
	if r.u32() != 0x616E6567 {
		fail("ungueltiger Actor-Block")
	}
	if v := r.u16(); v != 4 {
		fail("Actor-Block-Version %d", v)
	}
	end := int(r.u32()) + r.p
	if string(r.bytes(4)) != "FXA " {
		fail("FXA-Kennung fehlt")
	}
	r.skip(2)
	type rawSub struct {
		matID int
		pos   []float32
		nrm   []float32
		uv    []float32
		idx   []uint32
	}
	var subs []rawSub
	var matNames []string // Material-Chunks (Fallback)
	for r.p < end {
		id := r.u32()
		size := int(r.u32())
		ver := r.u32()
		cend := r.p + size
		if cend > end {
			fail("Chunk ueber Blockende")
		}
		switch {
		case id == 3 && ver == 3 && keep:
			r.u32() // Knoten
			r.u32()
			r.u32()
			r.u32()
			nsub := int(r.u32())
			nuv := int(r.u32())
			coll := r.u8()
			r.skip(3)
			for s := 0; s < nsub; s++ {
				var rs rawSub
				rs.matID = int(r.u8())
				r.u8()
				r.skip(2)
				ni := int(r.u32())
				nv := int(r.u32())
				vs := 28 + 8*nuv
				if nv < 0 || ni < 0 || nv*vs > len(r.d) {
					fail("unplausible Vertexanzahl")
				}
				rs.pos = make([]float32, 0, nv*3)
				rs.nrm = make([]float32, 0, nv*3)
				rs.uv = make([]float32, 0, nv*2)
				for v := 0; v < nv; v++ {
					r.u32()
					px, py, pz := r.f32(), r.f32(), r.f32()
					nx, ny, nz := r.f32(), r.f32(), r.f32()
					// EMotionFX speichert Z, Y, X; Gothic-Raum: (z, y, -x)
					rs.pos = append(rs.pos, pz, py, -px)
					rs.nrm = append(rs.nrm, nz, ny, -nx)
					if nuv > 0 {
						u, vv := r.f32(), r.f32()
						rs.uv = append(rs.uv, u, vv)
						r.skip(8 * (nuv - 1))
					} else {
						rs.uv = append(rs.uv, 0, 0)
					}
				}
				rs.idx = make([]uint32, ni)
				for k := range rs.idx {
					rs.idx[k] = r.u32()
				}
				// Dreiecksreihenfolge an die Spiegelung anpassen
				for k := 0; k+2 < len(rs.idx); k += 3 {
					rs.idx[k+1], rs.idx[k+2] = rs.idx[k+2], rs.idx[k+1]
				}
				if coll == 0 {
					subs = append(subs, rs)
				}
			}
		case id == 6 && ver == 5 && keep:
			r.skip(12 * 4) // vier Farben (RGB)
			r.skip(4 * 4)
			r.skip(4)
			name := string(r.bytes(int(r.u32())))
			matNames = append(matNames, name)
		}
		r.p = cend
	}
	r.p = end
	// Materialzuordnung: MaterialID -> Name
	nm := int(r.u32())
	mats := map[int]string{}
	for i := 0; i < nm; i++ {
		r.u16()
		mi := int(r.u16())
		mats[mi] = r.entry()
	}
	// Umgebungsverdeckung und Tangenten pro LoD ueberspringen
	r.skip(1)
	nl := int(r.u32())
	counts := make([]int, nl)
	for i := 0; i < nl; i++ {
		r.skip(1)
		c := int(r.u32())
		counts[i] = c
		r.skip(c * 4)
	}
	for _, c := range counts {
		r.skip(c * 12)
	}
	if !keep {
		return nil
	}
	m := &Mesh{Min: [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32},
		Max: [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}}
	for _, s := range subs {
		nv := len(s.pos) / 3
		if nv == 0 || len(s.idx) < 3 {
			continue
		}
		okIdx := true
		for _, ix := range s.idx {
			if int(ix) >= nv {
				okIdx = false
				break
			}
		}
		if !okIdx {
			continue
		}
		name := mats[s.matID]
		if name == "" && s.matID < len(matNames) {
			name = trimPipes(matNames[s.matID]) + ".xshmat"
		}
		sm := SubMesh{Material: name, Pos: s.pos, Normal: s.nrm, UV: s.uv, Index: s.idx[:len(s.idx)/3*3]}
		for v := 0; v < nv; v++ {
			for a := 0; a < 3; a++ {
				x := s.pos[v*3+a]
				m.Min[a] = min(m.Min[a], x)
				m.Max[a] = max(m.Max[a], x)
			}
		}
		m.Subs = append(m.Subs, sm)
	}
	if len(m.Subs) == 0 {
		m.Min, m.Max = [3]float32{}, [3]float32{}
	}
	return m
}

func trimPipes(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '|' {
			out = append(out, s[i])
		}
	}
	return string(out)
}
