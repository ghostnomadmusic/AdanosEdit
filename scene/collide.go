package scene

// Einfache Kollisionsabfrage gegen die Weltgeometrie (Gelaende, Gebaeude,
// Felsen ...), z. B. um ein Objekt auf den Boden zu setzen. Verwendet werden
// die Darstellungs-Meshes (nur Positionen), die bei Bedarf geladen und
// zwischengespeichert werden.

import (
	"math"
	"strings"
	"sync"

	"g3viewer/g3"
)

// Geom ist die reine Dreiecksgeometrie eines Modells (lokale Koordinaten).
type Geom struct {
	Pos      []float32
	Idx      []uint32
	Min, Max Vec3
}

var geomCache = struct {
	sync.Mutex
	m map[string]*Geom
}{m: map[string]*Geom{}}

// Geometry laedt die Geometrie eines (statischen) Modells; nil wenn keine.
func (l *Loader) Geometry(key string) *Geom {
	geomCache.Lock()
	g, ok := geomCache.m[key]
	geomCache.Unlock()
	if ok {
		return g
	}
	g = l.loadGeom(key)
	geomCache.Lock()
	if len(geomCache.m) > 3000 {
		geomCache.m = map[string]*Geom{}
	}
	geomCache.m[key] = g
	geomCache.Unlock()
	return g
}

func (l *Loader) loadGeom(key string) *Geom {
	if strings.HasPrefix(key, "tree|") {
		return nil
	}
	mesh, _ := splitKey(key)
	low := strings.ToLower(mesh)
	if strings.HasSuffix(low, ".fxa") || strings.HasSuffix(low, ".xact") {
		return nil
	}
	e := l.vfs.FindBase("_compiledMesh", mesh)
	if e == nil {
		return nil
	}
	d, err := e.Read()
	if err != nil {
		return nil
	}
	if strings.HasSuffix(low, ".xlmsh") {
		names, err := g3.ParseLodMesh(d)
		if err != nil || len(names) == 0 {
			return nil
		}
		if e = l.vfs.FindBase("_compiledMesh", names[0]); e == nil {
			return nil
		}
		if d, err = e.Read(); err != nil {
			return nil
		}
	}
	m, err := g3.ParseMesh(d)
	if err != nil {
		return nil
	}
	g := &Geom{Min: Vec3(m.Min), Max: Vec3(m.Max)}
	if meshSpecial(low) == SpecialBarrier {
		return nil // die Kuppel ist kein Boden
	}
	for _, s := range m.Subs {
		if mi := l.material(s.Material); mi != nil && mi.mat.Blend == g3.BlendInvisible {
			continue
		}
		if materialSpecial(s.Material) == SpecialBarrier {
			continue
		}
		base := uint32(len(g.Pos) / 3)
		g.Pos = append(g.Pos, s.Pos...)
		for _, i := range s.Index {
			g.Idx = append(g.Idx, i+base)
		}
	}
	if len(g.Idx) == 0 {
		return nil
	}
	return g
}

// rayGeom schneidet einen Strahl (lokale Koordinaten) mit der Geometrie.
func rayGeom(g *Geom, o, d Vec3) (float32, bool) {
	best := float32(math.MaxFloat32)
	hit := false
	p := g.Pos
	for i := 0; i+2 < len(g.Idx); i += 3 {
		a, b, c := g.Idx[i]*3, g.Idx[i+1]*3, g.Idx[i+2]*3
		if int(a)+2 >= len(p) || int(b)+2 >= len(p) || int(c)+2 >= len(p) {
			continue
		}
		// schnelle Ablehnung in X/Z (Strahl ist hier fast immer senkrecht)
		va := Vec3{p[a], p[a+1], p[a+2]}
		vb := Vec3{p[b], p[b+1], p[b+2]}
		vc := Vec3{p[c], p[c+1], p[c+2]}
		if t, ok := RayTriangle(o, d, va, vb, vc); ok && t < best {
			best, hit = t, true
		}
	}
	return best, hit
}

// GroundBelow sucht unterhalb von (x, startY, z) die erste Flaeche der
// Weltgeometrie und liefert ihre Hoehe. skip wird nicht beruecksichtigt.
func (w *World) GroundBelow(x, startY, z float32, skip int32, geom func(key string) *Geom) (float32, bool) {
	best := float32(-math.MaxFloat32)
	found := false
	seen := map[int32]bool{}
	w.Query(Vec3{x, 0, z}, 1, func(i int32) {
		if i == skip || seen[i] {
			return
		}
		seen[i] = true
		in := &w.Inst[i]
		if in.Hidden || in.ModelKey == "" || in.Marker != MarkNone {
			return
		}
		if x < in.Min[0] || x > in.Max[0] || z < in.Min[2] || z > in.Max[2] || in.Min[1] > startY {
			return
		}
		switch in.Ent.Kind {
		case g3.KindNPC, g3.KindAnimated, g3.KindTree, g3.KindItem:
			return
		}
		if in.Ent.Kind == g3.KindMesh && hasClass(in.Ent, "gCItem_PS") {
			return
		}
		g := geom(in.ModelKey)
		if g == nil {
			return
		}
		inv, ok := in.World.Inverse()
		if !ok {
			return
		}
		o := inv.TransformPoint(Vec3{x, startY, z})
		d := inv.TransformDir(Vec3{0, -1, 0})
		if t, ok := rayGeom(g, o, d); ok {
			y := startY - t
			if y > best {
				best, found = y, true
			}
		}
	})
	return best, found
}

func hasClass(e *g3.Entity, c string) bool {
	for _, x := range e.Classes {
		if x == c {
			return true
		}
	}
	return false
}
