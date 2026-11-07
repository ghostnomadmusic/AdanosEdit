package g3

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math"
)

// WorldFile ist eine bearbeitbare .lrentdat- oder .node-Datei. Geaendert
// werden nur die bekannten Felder (Matrizen, Boxen, GUID) direkt in den
// Originalbytes; alles andere bleibt unangetastet.
type WorldFile struct {
	Path     string
	Data     []byte
	IsLrent  bool
	Ents     []*Entity
	Raw      [][]byte // Bytes jeder Entity (werden beim Bearbeiten veraendert)
	Deleted  []bool
	Links    [][2]int32
	countOff int
	linksBeg int
	linksEnd int
	Dirty    bool
	Changed  map[int]bool // seit dem letzten Abgleich geaenderte Entities
	origRoot [24]byte     // urspruengliche Kontext-Box (zum Ersetzen in Begleitdateien)
	// Stringtabelle (neue Eintraege werden beim Speichern angehaengt)
	Table        []string
	strIdx       map[string]int
	tableChanged bool
}

var randRead = rand.Read

// OpenWorldFile liest eine Datei zum Bearbeiten (nur aktuelle Formate).
func OpenWorldFile(d []byte, path string) (wf *WorldFile, err error) {
	defer recoverParse(&err)
	if len(d) < 22 || string(d[:8]) != "GENOMFLE" {
		return nil, fmt.Errorf("dieses (alte) Dateiformat kann nicht bearbeitet werden")
	}
	r, _ := genome(d)
	wf = &WorldFile{Path: path, Data: d}
	wf.Table = append([]string(nil), r.table...)
	var ents []*Entity
	if string(d[14:22]) == "GENOMEDL" {
		wf.IsLrent = true
		r.p = 22
		if v := r.u16(); v != 0x53 {
			fail("lrentdat-Version 0x%X nicht unterstuetzt", v)
		}
		r.skip(6)
		r.entry()
		r.skip(5)
		r.u16()
		r.u32()
		r.u16()
		n := int(r.u32())
		for i := 0; i < n; i++ {
			r.entry()
			r.entry()
			r.u16()
			r.bytes(int(r.u32()))
		}
		r.u16()
		r.skip(33)
		ents, _ = r.entities(path, fmtLrentdat)
	} else {
		if v := r.u16(); v != 0x53 {
			fail("node-Version 0x%X nicht unterstuetzt", v)
		}
		ents, _ = r.entities(path, fmtNode)
	}
	wf.Ents = ents
	wf.countOff = r.countOff
	wf.linksBeg = r.linksStart
	wf.linksEnd = r.linksEnd
	wf.Links = r.links
	for _, e := range ents {
		wf.Raw = append(wf.Raw, append([]byte(nil), d[e.Span.Start:e.Span.End]...))
		wf.Deleted = append(wf.Deleted, false)
	}
	if len(ents) > 0 {
		copy(wf.origRoot[:], wf.Raw[0][ents[0].Span.Matrix-ents[0].Span.Start+128:])
	}
	return wf, nil
}

func getF(b []byte, o int) float32    { return math.Float32frombits(binary.LittleEndian.Uint32(b[o:])) }
func putF(b []byte, o int, v float32) { binary.LittleEndian.PutUint32(b[o:], math.Float32bits(v)) }

// Matrix liest eine 4x4-Matrix (Zeilenvektoren) aus den Rohdaten.
func (wf *WorldFile) matrix(i int, local bool) [16]float32 {
	var m [16]float32
	o := wf.Ents[i].Span.Matrix - wf.Ents[i].Span.Start
	if local {
		o += 64
	}
	for k := range m {
		m[k] = getF(wf.Raw[i], o+k*4)
	}
	return m
}

func (wf *WorldFile) setMatrix(i int, local bool, m [16]float32) {
	o := wf.Ents[i].Span.Matrix - wf.Ents[i].Span.Start
	if local {
		o += 64
	}
	for k := range m {
		putF(wf.Raw[i], o+k*4, m[k])
	}
}

// Mul fuer Zeilenvektor-Matrizen: erst a, dann b.
func MatMul(a, b [16]float32) [16]float32 {
	var r [16]float32
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			var s float32
			for k := 0; k < 4; k++ {
				s += a[i*4+k] * b[k*4+j]
			}
			r[i*4+j] = s
		}
	}
	return r
}

func transformPoint(m [16]float32, p [3]float32) [3]float32 {
	return [3]float32{
		p[0]*m[0] + p[1]*m[4] + p[2]*m[8] + m[12],
		p[0]*m[1] + p[1]*m[5] + p[2]*m[9] + m[13],
		p[0]*m[2] + p[1]*m[6] + p[2]*m[10] + m[14],
	}
}

// TransformBox bildet eine lokale Box mit einer Matrix auf eine Welt-AABB ab.
func TransformBox(m [16]float32, mn, mx [3]float32) ([3]float32, [3]float32) {
	omn := [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}
	omx := [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}
	for c := 0; c < 8; c++ {
		p := mn
		if c&1 != 0 {
			p[0] = mx[0]
		}
		if c&2 != 0 {
			p[1] = mx[1]
		}
		if c&4 != 0 {
			p[2] = mx[2]
		}
		q := transformPoint(m, p)
		for a := 0; a < 3; a++ {
			omn[a] = min(omn[a], q[a])
			omx[a] = max(omx[a], q[a])
		}
	}
	return omn, omx
}

func (wf *WorldFile) box(i, which int) ([3]float32, [3]float32) {
	o := wf.Ents[i].Span.Matrix - wf.Ents[i].Span.Start + 128 + which*24
	b := wf.Raw[i]
	return [3]float32{getF(b, o), getF(b, o+4), getF(b, o+8)}, [3]float32{getF(b, o+12), getF(b, o+16), getF(b, o+20)}
}

func (wf *WorldFile) setBox(i, which int, mn, mx [3]float32) {
	o := wf.Ents[i].Span.Matrix - wf.Ents[i].Span.Start + 128 + which*24
	for a := 0; a < 3; a++ {
		putF(wf.Raw[i], o+a*4, mn[a])
		putF(wf.Raw[i], o+12+a*4, mx[a])
	}
}

func (wf *WorldFile) setSphere(i, which int, mn, mx [3]float32) {
	o := wf.Ents[i].Span.Matrix - wf.Ents[i].Span.Start + 128 + 72 + which*16
	if !BoxValid(mn, mx) {
		putF(wf.Raw[i], o, 0)
		return
	}
	c := [3]float32{(mn[0] + mx[0]) / 2, (mn[1] + mx[1]) / 2, (mn[2] + mx[2]) / 2}
	dx, dy, dz := mx[0]-mn[0], mx[1]-mn[1], mx[2]-mn[2]
	r := float32(math.Sqrt(float64(dx*dx+dy*dy+dz*dz))) / 2
	putF(wf.Raw[i], o, r)
	for a := 0; a < 3; a++ {
		putF(wf.Raw[i], o+4+a*4, c[a])
	}
}

func (wf *WorldFile) children(i int) []int {
	var out []int
	for _, l := range wf.Links {
		if int(l[0]) == i && !wf.Deleted[l[1]] {
			out = append(out, int(l[1]))
		}
	}
	return out
}

func (wf *WorldFile) parent(i int) int {
	for _, l := range wf.Links {
		if int(l[1]) == i {
			return int(l[0])
		}
	}
	return -1
}

// updateNode berechnet Weltmatrix, Knotenboxen und Kugeln einer Entity neu
// (aus lokaler Matrix und lokaler Box) und rekursiv fuer die Kinder.
func (wf *WorldFile) mark(i int) {
	if wf.Changed == nil {
		wf.Changed = map[int]bool{}
	}
	wf.Changed[i] = true
}

func (wf *WorldFile) updateNode(i int, world [16]float32) {
	wf.mark(i)
	wf.setMatrix(i, false, world)
	wf.Ents[i].World = world
	lmn, lmx := wf.box(i, 1)
	if BoxValid(lmn, lmx) {
		nmn, nmx := TransformBox(world, lmn, lmx)
		wf.setBox(i, 2, nmn, nmx)
		wf.setSphere(i, 1, nmn, nmx)
		wf.Ents[i].NodeMin, wf.Ents[i].NodeMax = nmn, nmx
		if v := wf.Ents[i].Span.Vis; v >= 0 {
			o := v - wf.Ents[i].Span.Start
			for a := 0; a < 3; a++ {
				putF(wf.Raw[i], o+a*4, nmn[a])
				putF(wf.Raw[i], o+12+a*4, nmx[a])
			}
			// orientierte Box: Mitte, halbe Ausdehnung, Drehung (3x3)
			lc := [3]float32{(lmn[0] + lmx[0]) / 2, (lmn[1] + lmx[1]) / 2, (lmn[2] + lmx[2]) / 2}
			c := transformPoint(world, lc)
			ob := o + 24
			for a := 0; a < 3; a++ {
				putF(wf.Raw[i], ob+a*4, c[a])
			}
			for row := 0; row < 3; row++ {
				l := float32(math.Sqrt(float64(world[row*4]*world[row*4] + world[row*4+1]*world[row*4+1] + world[row*4+2]*world[row*4+2])))
				putF(wf.Raw[i], ob+12+row*4, (lmx[row]-lmn[row])/2*l)
				for col := 0; col < 3; col++ {
					v := world[row*4+col]
					if l > 0 {
						v /= l
					}
					putF(wf.Raw[i], ob+24+row*12+col*4, v)
				}
			}
		}
	}
	for _, c := range wf.children(i) {
		local := wf.matrix(c, true)
		wf.updateNode(c, MatMul(local, world))
	}
	wf.updateTree(i)
}

// updateTree setzt die Teilbaum-Box aus Knotenbox und Kindern.
func (wf *WorldFile) updateTree(i int) {
	mn, mx := wf.box(i, 2)
	if !BoxValid(mn, mx) {
		mn = [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}
		mx = [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}
	}
	for _, c := range wf.children(i) {
		cmn, cmx := wf.box(c, 0)
		if !BoxValid(cmn, cmx) {
			continue
		}
		for a := 0; a < 3; a++ {
			mn[a] = min(mn[a], cmn[a])
			mx[a] = max(mx[a], cmx[a])
		}
	}
	wf.setBox(i, 0, mn, mx)
	wf.setSphere(i, 0, mn, mx)
	wf.Ents[i].TreeMin, wf.Ents[i].TreeMax = mn, mx
	wf.mark(i)
}

// SetWorld setzt die Weltmatrix einer Entity (Kinder wandern mit).
func (wf *WorldFile) SetWorld(i int, world [16]float32) {
	p := wf.parent(i)
	local := world
	if p >= 0 {
		pw := wf.matrix(p, false)
		if inv, ok := invert(pw); ok {
			local = MatMul(world, inv)
		}
	}
	wf.setMatrix(i, true, local)
	wf.updateNode(i, world)
	for p >= 0 {
		wf.updateTree(p)
		p = wf.parent(p)
	}
	wf.Dirty = true
}

// Delete entfernt eine Entity samt Kindern. Die Wurzel kann nicht geloescht werden.
func (wf *WorldFile) Delete(i int) error {
	if i == 0 {
		return fmt.Errorf("die Wurzel einer Datei kann nicht geloescht werden")
	}
	var del func(int)
	del = func(k int) {
		for _, c := range wf.children(k) {
			del(c)
		}
		wf.Deleted[k] = true
		wf.mark(k)
	}
	del(i)
	if p := wf.parent(i); p >= 0 {
		for ; p >= 0; p = wf.parent(p) {
			wf.updateTree(p)
		}
	}
	wf.Dirty = true
	return nil
}

// Undelete stellt eine geloeschte Entity samt Kindern wieder her.
func (wf *WorldFile) Undelete(i int) {
	var und func(int)
	und = func(k int) {
		wf.Deleted[k] = false
		wf.mark(k)
		for _, l := range wf.Links {
			if int(l[0]) == k {
				und(int(l[1]))
			}
		}
	}
	und(i)
	for p := wf.parent(i); p >= 0; p = wf.parent(p) {
		wf.updateTree(p)
	}
	wf.Dirty = true
}

// Clone legt eine Kopie einer Entity (ohne Kinder) unter der Wurzel an und
// liefert ihren Index. Die Kopie bekommt eine neue GUID.
func (wf *WorldFile) Clone(i int) (int, error) {
	if len(wf.children(i)) > 0 {
		return -1, fmt.Errorf("Objekte mit Unterobjekten (z. B. NPCs) koennen nicht kopiert werden")
	}
	if i == 0 {
		return -1, fmt.Errorf("die Wurzel kann nicht kopiert werden")
	}
	src := wf.Ents[i]
	raw := append([]byte(nil), wf.Raw[i]...)
	var g [16]byte
	randRead(g[:])
	g[7] = g[7]&0x0f | 0x40
	g[8] = g[8]&0x3f | 0x80
	copy(raw[src.Span.GUID-src.Span.Start:], g[:])
	ne := *src
	ne.GUID = g
	ne.Index = len(wf.Ents)
	wf.Ents = append(wf.Ents, &ne)
	wf.Raw = append(wf.Raw, raw)
	wf.Deleted = append(wf.Deleted, false)
	wf.Links = append(wf.Links, [2]int32{0, int32(ne.Index)})
	// lokale Matrix relativ zur Wurzel
	world := wf.matrix(ne.Index, false)
	wf.SetWorld(ne.Index, world)
	return ne.Index, nil
}

// RootBox liefert die Teilbaum-Box der Wurzel.
func (wf *WorldFile) RootBox() ([3]float32, [3]float32) { return wf.box(0, 0) }

// Bytes erzeugt die geaenderte Datei.
func (wf *WorldFile) Bytes() []byte {
	remap := make([]int32, len(wf.Ents))
	n := 0
	for i := range wf.Ents {
		if wf.Deleted[i] {
			remap[i] = -1
			continue
		}
		remap[i] = int32(n)
		n++
	}
	var out bytes.Buffer
	out.Write(wf.Data[:wf.countOff])
	binary.Write(&out, binary.LittleEndian, int32(n))
	for i := range wf.Ents {
		if !wf.Deleted[i] {
			out.Write(wf.Raw[i])
		}
	}
	for _, l := range wf.Links {
		a, b := remap[l[0]], remap[l[1]]
		if a < 0 || b < 0 {
			continue
		}
		binary.Write(&out, binary.LittleEndian, a)
		binary.Write(&out, binary.LittleEndian, b)
	}
	binary.Write(&out, binary.LittleEndian, int32(-1))
	binary.Write(&out, binary.LittleEndian, int32(-1))
	tabOld := int(binary.LittleEndian.Uint32(wf.Data[10:14]))
	tabNew := out.Len() + (tabOld - wf.linksEnd)
	if wf.tableChanged {
		out.Write(wf.Data[wf.linksEnd : tabOld+4])
		// alte Tabelle ueberspringen
		p := tabOld + 4
		if p < len(wf.Data) && wf.Data[p] != 0 {
			n := int(binary.LittleEndian.Uint32(wf.Data[p+1:]))
			p += 5
			for i := 0; i < n; i++ {
				p += 2 + int(binary.LittleEndian.Uint16(wf.Data[p:]))
			}
		} else {
			p++
		}
		out.WriteByte(1)
		binary.Write(&out, binary.LittleEndian, uint32(len(wf.Table)))
		for _, s := range wf.Table {
			binary.Write(&out, binary.LittleEndian, uint16(len(s)))
			out.WriteString(s)
		}
		out.Write(wf.Data[p:])
	} else {
		out.Write(wf.Data[wf.linksEnd:])
	}
	b := out.Bytes()
	binary.LittleEndian.PutUint32(b[10:14], uint32(tabNew))
	if wf.IsLrent {
		// Groesse des Kontext-Objekts reicht bis zur Stringtabelle
		binary.LittleEndian.PutUint32(b[39:43], uint32(tabNew-43))
		mn, mx := wf.RootBox()
		replaceBox(b[:wf.countOff], wf.origRoot, mn, mx)
	}
	return b
}

// replaceBox ersetzt alle Vorkommen einer Box (24 Byte) durch eine neue.
func replaceBox(b []byte, old [24]byte, mn, mx [3]float32) int {
	var nb [24]byte
	for a := 0; a < 3; a++ {
		binary.LittleEndian.PutUint32(nb[a*4:], math.Float32bits(mn[a]))
		binary.LittleEndian.PutUint32(nb[12+a*4:], math.Float32bits(mx[a]))
	}
	n := 0
	for i := 0; i+24 <= len(b); {
		j := bytes.Index(b[i:], old[:])
		if j < 0 {
			break
		}
		copy(b[i+j:], nb[:])
		i += j + 24
		n++
	}
	return n
}

// UpdateGeoContext passt die Kontext-Box einer .lrgeodat-Datei an.
func (wf *WorldFile) UpdateGeoContext(geo []byte) []byte {
	out := append([]byte(nil), geo...)
	mn, mx := wf.RootBox()
	replaceBox(out, wf.origRoot, mn, mx)
	return out
}

func invert(m [16]float32) ([16]float32, bool) {
	// allgemeine 4x4-Inverse (Zeilen- oder Spaltenkonvention egal)
	var a [16]float64
	for i := range a {
		a[i] = float64(m[i])
	}
	var inv [16]float64
	inv[0] = a[5]*a[10]*a[15] - a[5]*a[11]*a[14] - a[9]*a[6]*a[15] + a[9]*a[7]*a[14] + a[13]*a[6]*a[11] - a[13]*a[7]*a[10]
	inv[4] = -a[4]*a[10]*a[15] + a[4]*a[11]*a[14] + a[8]*a[6]*a[15] - a[8]*a[7]*a[14] - a[12]*a[6]*a[11] + a[12]*a[7]*a[10]
	inv[8] = a[4]*a[9]*a[15] - a[4]*a[11]*a[13] - a[8]*a[5]*a[15] + a[8]*a[7]*a[13] + a[12]*a[5]*a[11] - a[12]*a[7]*a[9]
	inv[12] = -a[4]*a[9]*a[14] + a[4]*a[10]*a[13] + a[8]*a[5]*a[14] - a[8]*a[6]*a[13] - a[12]*a[5]*a[10] + a[12]*a[6]*a[9]
	inv[1] = -a[1]*a[10]*a[15] + a[1]*a[11]*a[14] + a[9]*a[2]*a[15] - a[9]*a[3]*a[14] - a[13]*a[2]*a[11] + a[13]*a[3]*a[10]
	inv[5] = a[0]*a[10]*a[15] - a[0]*a[11]*a[14] - a[8]*a[2]*a[15] + a[8]*a[3]*a[14] + a[12]*a[2]*a[11] - a[12]*a[3]*a[10]
	inv[9] = -a[0]*a[9]*a[15] + a[0]*a[11]*a[13] + a[8]*a[1]*a[15] - a[8]*a[3]*a[13] - a[12]*a[1]*a[11] + a[12]*a[3]*a[9]
	inv[13] = a[0]*a[9]*a[14] - a[0]*a[10]*a[13] - a[8]*a[1]*a[14] + a[8]*a[2]*a[13] + a[12]*a[1]*a[10] - a[12]*a[2]*a[9]
	inv[2] = a[1]*a[6]*a[15] - a[1]*a[7]*a[14] - a[5]*a[2]*a[15] + a[5]*a[3]*a[14] + a[13]*a[2]*a[7] - a[13]*a[3]*a[6]
	inv[6] = -a[0]*a[6]*a[15] + a[0]*a[7]*a[14] + a[4]*a[2]*a[15] - a[4]*a[3]*a[14] - a[12]*a[2]*a[7] + a[12]*a[3]*a[6]
	inv[10] = a[0]*a[5]*a[15] - a[0]*a[7]*a[13] - a[4]*a[1]*a[15] + a[4]*a[3]*a[13] + a[12]*a[1]*a[7] - a[12]*a[3]*a[5]
	inv[14] = -a[0]*a[5]*a[14] + a[0]*a[6]*a[13] + a[4]*a[1]*a[14] - a[4]*a[2]*a[13] - a[12]*a[1]*a[6] + a[12]*a[2]*a[5]
	inv[3] = -a[1]*a[6]*a[11] + a[1]*a[7]*a[10] + a[5]*a[2]*a[11] - a[5]*a[3]*a[10] - a[9]*a[2]*a[7] + a[9]*a[3]*a[6]
	inv[7] = a[0]*a[6]*a[11] - a[0]*a[7]*a[10] - a[4]*a[2]*a[11] + a[4]*a[3]*a[10] + a[8]*a[2]*a[7] - a[8]*a[3]*a[6]
	inv[11] = -a[0]*a[5]*a[11] + a[0]*a[7]*a[9] + a[4]*a[1]*a[11] - a[4]*a[3]*a[9] - a[8]*a[1]*a[7] + a[8]*a[3]*a[5]
	inv[15] = a[0]*a[5]*a[10] - a[0]*a[6]*a[9] - a[4]*a[1]*a[10] + a[4]*a[2]*a[9] + a[8]*a[1]*a[6] - a[8]*a[2]*a[5]
	det := a[0]*inv[0] + a[1]*inv[4] + a[2]*inv[8] + a[3]*inv[12]
	if math.Abs(det) < 1e-20 {
		return m, false
	}
	var r [16]float32
	for i := range inv {
		r[i] = float32(inv[i] / det)
	}
	return r, true
}

// RawCopy liefert eine Kopie der Rohdaten einer Entity (fuer Rueckgaengig).
func (wf *WorldFile) RawCopy(i int) []byte { return append([]byte(nil), wf.Raw[i]...) }

// RestoreRaw setzt die Rohdaten einer Entity zurueck.
func (wf *WorldFile) RestoreRaw(i int, b []byte) {
	wf.Raw[i] = append([]byte(nil), b...)
	wf.mark(i)
	wf.Dirty = true
}
