package scene

import (
	"math"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"g3viewer/g3"
)

// Instance ist ein darstellbares Objekt der Welt.
type Instance struct {
	Ent      *g3.Entity
	Min, Max Vec3 // Welt-AABB
	Center   Vec3
	Radius   float32
	ModelKey string // "" = Platzhalter (Marker)
	Model    int32  // Index in die Modell-Tabelle des Renderers, -1 = keins
	Frame    uint32 // zuletzt gezeichnet (Doppelzeichnung vermeiden)
	Marker   MarkerKind
	World    Mat4
	Sector   string
	Hidden   bool // geloescht (im Editor)
}

// MarkerKind bestimmt Farbe und Sichtbarkeit von Platzhaltern.
type MarkerKind int

const (
	MarkNone MarkerKind = iota
	MarkNPC
	MarkItem
	MarkTree
	MarkLight
	MarkAnimated
	MarkHelper
	MarkZone  // Zone (gCNavZone_PS), z. B. fuer "Ort erreichen"
	MarkArena // Arena-Zone (gCArena_PS)
	MarkPoint // Routinepunkt / freier Platz (gCAIHelper_FreePoint_PS)
)

// IsHelperMark: Hilfsobjekt (mit dem Haekchen "Hilfsobjekte" schaltbar).
func IsHelperMark(k MarkerKind) bool {
	return k == MarkHelper || k == MarkZone || k == MarkArena || k == MarkPoint
}

// World ist der Index aller Objekte mit raeumlichem Gitter.
type World struct {
	Inst     []Instance
	cellSize float32
	grid     map[int64][]int32
	Min, Max Vec3
	Errors   []string
	Files    int
}

const cellSize = 4000 // 40 m

func cellKey(ix, iz int32) int64 { return int64(ix)<<32 | int64(uint32(iz)) }

func (w *World) cellRange(mn, mx Vec3) (int32, int32, int32, int32) {
	f := func(v float32) int32 { return int32(math.Floor(float64(v / w.cellSize))) }
	return f(mn[0]), f(mn[2]), f(mx[0]), f(mx[2])
}

// Query ruft fn fuer alle Instanzen auf, deren Zellen den Kreis (XZ) beruehren.
// Instanzen koennen mehrfach geliefert werden.
func (w *World) Query(c Vec3, radius float32, fn func(i int32)) {
	x0, z0, x1, z1 := w.cellRange(Vec3{c[0] - radius, 0, c[2] - radius}, Vec3{c[0] + radius, 0, c[2] + radius})
	if (x1-x0+1)*(z1-z0+1) > 400000 {
		return
	}
	for ix := x0; ix <= x1; ix++ {
		for iz := z0; iz <= z1; iz++ {
			for _, i := range w.grid[cellKey(ix, iz)] {
				fn(i)
			}
		}
	}
}

// excluded: Sektoren, die nicht angezeigt werden (Fernsicht-Ersatzmodelle).
func excluded(path string) bool {
	l := strings.ToLower(path)
	return strings.Contains(l, "lowpoly")
}

func isRootLike(e *g3.Entity) bool {
	if len(e.Classes) == 0 {
		return true
	}
	for _, c := range e.Classes {
		// Effekt-Geometrie, die im Spiel unsichtbar oder nur als Shader-Effekt erscheint
		if c == "gCMagicBarrier_PS" {
			return true
		}
	}
	return false
}

func markerFor(e *g3.Entity) MarkerKind {
	switch e.Kind {
	case g3.KindNPC:
		return MarkNPC
	case g3.KindItem:
		return MarkItem
	case g3.KindTree:
		return MarkTree
	case g3.KindLight:
		return MarkLight
	case g3.KindAnimated:
		return MarkAnimated
	}
	has := func(c string) bool {
		for _, x := range e.Classes {
			if x == c {
				return true
			}
		}
		return false
	}
	switch {
	case has("gCArena_PS"):
		return MarkArena
	case has("gCNavZone_PS"):
		return MarkZone
	case has("gCAIHelper_FreePoint_PS"):
		return MarkPoint
	}
	return MarkHelper
}

// BuildWorld liest alle Weltdateien aus Projects_compiled.
func BuildWorld(v *g3.VFS, progress func(done, total int)) *World {
	var files []*g3.PakEntry
	for _, e := range v.List("Projects_compiled") {
		n := strings.ToLower(e.Name)
		if !strings.HasPrefix(n, "g3_world_01/") {
			continue
		}
		if !strings.HasSuffix(n, ".node") && !strings.HasSuffix(n, ".lrentdat") {
			continue
		}
		if excluded(n) {
			continue
		}
		files = append(files, e)
	}
	type res struct {
		idx  int
		ents []*g3.Entity
		err  string
	}
	results := make([]res, len(files))
	var next, done int64
	var wg sync.WaitGroup
	workers := runtime.NumCPU()
	if workers > 8 {
		workers = 8
	}
	if workers < 1 {
		workers = 1
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(atomic.AddInt64(&next, 1) - 1)
				if i >= len(files) {
					return
				}
				f := files[i]
				d, err := f.Read()
				if err == nil {
					var ents []*g3.Entity
					ents, err = g3.ParseWorldFile(d, f.Name)
					results[i] = res{idx: i, ents: ents}
				}
				if err != nil && !strings.Contains(f.Name, "{") {
					results[i].err = f.Name + ": " + err.Error()
				}
				n := atomic.AddInt64(&done, 1)
				if progress != nil && (n%64 == 0 || int(n) == len(files)) {
					progress(int(n), len(files))
				}
			}
		}()
	}
	wg.Wait()

	w := &World{cellSize: cellSize, grid: map[int64][]int32{}, Files: len(files)}
	w.Min = Vec3{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}
	w.Max = Vec3{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}
	for i, r := range results {
		if r.err != "" {
			w.Errors = append(w.Errors, r.err)
		}
		sector := files[i].Name
		if j := strings.LastIndex(sector, "/"); j >= 0 {
			sector = sector[:j]
		}
		sector = strings.TrimPrefix(sector, "G3_World_01/")
		for _, e := range r.ents {
			if isRootLike(e) {
				continue
			}
			inst, ok := MakeInstance(e, sector)
			if !ok {
				continue
			}
			for a := 0; a < 3; a++ {
				if inst.Min[a] < w.Min[a] {
					w.Min[a] = inst.Min[a]
				}
				if inst.Max[a] > w.Max[a] {
					w.Max[a] = inst.Max[a]
				}
			}
			w.Inst = append(w.Inst, inst)
		}
	}
	for i := range w.Inst {
		in := &w.Inst[i]
		x0, z0, x1, z1 := w.cellRange(in.Min, in.Max)
		if (x1-x0+1)*(z1-z0+1) > 4096 {
			// riesige Objekte (z. B. Ozean) nur in die Zelle ihres Mittelpunkts
			x0, z0, x1, z1 = w.cellRange(in.Center, in.Center)
		}
		for ix := x0; ix <= x1; ix++ {
			for iz := z0; iz <= z1; iz++ {
				k := cellKey(ix, iz)
				w.grid[k] = append(w.grid[k], int32(i))
			}
		}
	}
	return w
}

// Search liefert Instanzen, deren Name den Text enthaelt (max. limit).
func (w *World) Search(text string, limit int) []int32 {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return nil
	}
	var out []int32
	for i := range w.Inst {
		if strings.Contains(strings.ToLower(w.Inst[i].Ent.Name), text) ||
			(w.Inst[i].Ent.Mesh != "" && strings.Contains(strings.ToLower(w.Inst[i].Ent.Mesh), text)) {
			out = append(out, int32(i))
			if len(out) >= limit {
				break
			}
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		return strings.ToLower(w.Inst[out[a]].Ent.Name) < strings.ToLower(w.Inst[out[b]].Ent.Name)
	})
	return out
}

// sane: Gothic-3-Welt liegt innerhalb von etwa +-5 km.
func sane(v Vec3) bool {
	for _, x := range v {
		if x != x || x < -1e6 || x > 1e6 {
			return false
		}
	}
	return true
}

// treeMatrix bildet den Einheitsbaum (x,z -0.5..0.5, y 0..1) auf die Box ab,
// mit der Drehung um Y aus der Weltmatrix.
func treeMatrix(e *g3.Entity, mn, mx Vec3) Mat4 {
	yaw := math.Atan2(float64(e.World[8]), float64(e.World[10]))
	c, s := float32(math.Cos(yaw)), float32(math.Sin(yaw))
	w, h, d := mx[0]-mn[0], mx[1]-mn[1], mx[2]-mn[2]
	sz := (w + d) / 2
	cx, cz := (mn[0]+mx[0])/2, (mn[2]+mx[2])/2
	// Spalten: X-Achse, Y-Achse, Z-Achse, Position
	return Mat4{
		c * sz, 0, -s * sz, 0,
		0, h, 0, 0,
		s * sz, 0, c * sz, 0,
		cx, mn[1], cz, 1,
	}
}

// MakeInstance erzeugt die Darstellung einer Entity (ok=false: nicht darstellbar).
func MakeInstance(e *g3.Entity, sector string) (Instance, bool) {
	inst := Instance{Ent: e, Model: -1, World: Mat4(e.World), Sector: sector}
	p := Vec3(e.Pos())
	switch {
	case e.Kind == g3.KindMesh:
		inst.ModelKey = ModelKey(e.Mesh, e.MatSwitch)
		if g3.BoxValid(e.NodeMin, e.NodeMax) {
			inst.Min, inst.Max = Vec3(e.NodeMin), Vec3(e.NodeMax)
		} else {
			inst.Min, inst.Max = p.Sub(Vec3{100, 100, 100}), p.Add(Vec3{100, 100, 100})
		}
	default:
		inst.Marker = markerFor(e)
		s := Vec3{40, 40, 40}
		off := Vec3{}
		switch inst.Marker {
		case MarkNPC:
			s, off = Vec3{35, 90, 35}, Vec3{0, 90, 0}
		case MarkTree:
			s, off = Vec3{60, 400, 60}, Vec3{0, 400, 0}
		case MarkLight:
			s = Vec3{20, 20, 20}
		case MarkItem:
			s = Vec3{15, 15, 15}
		}
		c := p.Add(off)
		inst.Min, inst.Max = c.Sub(s), c.Add(s)
		low := strings.ToLower(e.Mesh)
		switch {
		case strings.HasSuffix(low, ".fxa") || strings.HasSuffix(low, ".xact"):
			// Figur: Modell aus _compiledAnimation, Platzhalter nur als Ersatz
			inst.ModelKey = ModelKey(e.Mesh, e.MatSwitch)
			grow := Vec3{120, 0, 120}
			inst.Min, inst.Max = inst.Min.Sub(grow), inst.Max.Add(grow)
		case inst.Marker == MarkTree && strings.HasSuffix(low, ".spt"):
			inst.ModelKey = "tree|" + low
			if g3.BoxValid(e.NodeMin, e.NodeMax) {
				inst.Min, inst.Max = Vec3(e.NodeMin), Vec3(e.NodeMax)
			}
			inst.World = treeMatrix(e, inst.Min, inst.Max)
		}
	}
	inst.Center = inst.Min.Add(inst.Max).Scale(0.5)
	inst.Radius = inst.Max.Sub(inst.Min).Len() * 0.5
	if !sane(inst.Min) || !sane(inst.Max) {
		return inst, false // kaputte oder unbenutzte Objekte weit ausserhalb der Welt
	}
	return inst, true
}

func (w *World) gridAdd(i int32) {
	in := &w.Inst[i]
	x0, z0, x1, z1 := w.cellRange(in.Min, in.Max)
	if (x1-x0+1)*(z1-z0+1) > 4096 {
		x0, z0, x1, z1 = w.cellRange(in.Center, in.Center)
	}
	for ix := x0; ix <= x1; ix++ {
		for iz := z0; iz <= z1; iz++ {
			k := cellKey(ix, iz)
			w.grid[k] = append(w.grid[k], i)
		}
	}
}

func (w *World) gridRemove(i int32) {
	in := &w.Inst[i]
	x0, z0, x1, z1 := w.cellRange(in.Min, in.Max)
	if (x1-x0+1)*(z1-z0+1) > 4096 {
		x0, z0, x1, z1 = w.cellRange(in.Center, in.Center)
	}
	for ix := x0; ix <= x1; ix++ {
		for iz := z0; iz <= z1; iz++ {
			k := cellKey(ix, iz)
			l := w.grid[k]
			for j := 0; j < len(l); j++ {
				if l[j] == i {
					l[j] = l[len(l)-1]
					l = l[:len(l)-1]
					j--
				}
			}
			w.grid[k] = l
		}
	}
}

// Update ersetzt die Entity einer Instanz (nach dem Bearbeiten) und
// aktualisiert Lage, Box und Gitter. Modell und Sichtbarkeit bleiben.
func (w *World) Update(i int32, e *g3.Entity) {
	old := &w.Inst[i]
	w.gridRemove(i)
	ni, ok := MakeInstance(e, old.Sector)
	if !ok {
		old.Hidden = true
		return
	}
	ni.Model = old.Model
	ni.Hidden = old.Hidden
	w.Inst[i] = ni
	w.gridAdd(i)
}

// Add fuegt eine neue Instanz hinzu und liefert ihren Index.
func (w *World) Add(e *g3.Entity, sector string) int32 {
	ni, ok := MakeInstance(e, sector)
	if !ok {
		return -1
	}
	w.Inst = append(w.Inst, ni)
	i := int32(len(w.Inst) - 1)
	w.gridAdd(i)
	return i
}
