package g3

import (
	"bytes"
	"fmt"
	"strings"
)

// EntityKind grob eingeteilt, fuer Darstellung und Filter.
type EntityKind int

const (
	KindOther    EntityKind = iota
	KindMesh                // statisches/dynamisches Mesh (.xcmsh/.xlmsh)
	KindNPC                 // NPC oder Monster
	KindAnimated            // animiertes Objekt (.fxa), kein NPC
	KindTree                // SpeedTree
	KindLight               // Lichtquelle
	KindItem                // Gegenstand ohne Mesh
)

// Entity ist ein Objekt aus einer Sektor-Datei (.lrentdat/.node).
type Entity struct {
	Name      string
	GUID      [16]byte
	World     [16]float32 // Zeilen: X-Achse, Y-Achse, Z-Achse, Position (DirectX-Konvention)
	Mesh      string      // Dateiname des Meshes, falls vorhanden
	MatSwitch int
	Kind      EntityKind
	Classes   []string
	Source    string // Datei, aus der die Entity stammt
	// Begrenzungsboxen aus der Datei (Weltkoordinaten): Knoten und ganzer Teilbaum.
	NodeMin, NodeMax [3]float32
	TreeMin, TreeMax [3]float32
	// Lage in der Datei (fuer das Bearbeiten), nur bei aktuellen Formaten gesetzt
	Index int
	Span  Span
}

// Span beschreibt die Byte-Positionen einer Entity in ihrer Datei.
type Span struct {
	Start, End int // ganze Entity
	GUID       int // 16 Byte GUID (+4 Gueltig-Flag)
	Matrix     int // Weltmatrix (64 Byte), danach lokale Matrix, 3 Boxen, 2 Kugeln
	Vis        int // nur .node: sichtbare Weltbox (24) + orientierte Box (60); sonst -1
	Valid      bool
}

// BoxValid prueft, ob eine Box gesetzt ist.
func BoxValid(mn, mx [3]float32) bool {
	return mn[0] <= mx[0] && mn[1] <= mx[1] && mn[2] <= mx[2] && mx[0]-mn[0] < 1e7 && mx[1]-mn[1] < 1e7 && mx[2]-mn[2] < 1e7
}

// Pos liefert die Weltposition.
func (e *Entity) Pos() [3]float32 { return [3]float32{e.World[12], e.World[13], e.World[14]} }

// GUIDString formatiert die GUID wie im Editor.
func (e *Entity) GUIDString() string {
	g := e.GUID
	return fmt.Sprintf("{%02X%02X%02X%02X-%02X%02X-%02X%02X-%02X%02X-%02X%02X%02X%02X%02X%02X}",
		g[3], g[2], g[1], g[0], g[5], g[4], g[7], g[6], g[8], g[9], g[10], g[11], g[12], g[13], g[14], g[15])
}

// readClassNew liest ein Property-Set im aktuellen Format (Version 0x53 Entities).
func (r *reader) readClassNew() PropSet {
	r.u16()   // Klassenversion
	r.skip(6) // 01 00 01 01 00 01
	name := r.entry()
	r.skip(5) // 01 00 00 53 00
	ps := PropSet{Class: name}
	end := r.propsBody(&ps)
	if end < r.p || end > len(r.d) {
		fail("Klasse %s: ungueltige Groesse", name)
	}
	r.p = end
	if r.u32() != 0xDEADC0DE {
		fail("Klasse %s: Endmarke fehlt", name)
	}
	return ps
}

var deadcode = []byte{0xDE, 0xC0, 0xAD, 0xDE}

// readClassOld liest ein Property-Set im alten Format (mit Vorab-Groesse und Typname).
func (r *reader) readClassOld() PropSet {
	r.u16()
	pre := int(r.u32())
	a := r.p
	r.entry()
	r.skip(6)
	name := r.entry()
	r.skip(5)
	ps := PropSet{Class: name}
	r.propsBody(&ps)
	lim := a + pre + 64
	if lim > len(r.d) {
		lim = len(r.d)
	}
	from := r.p - 2
	if from < a {
		from = a
	}
	j := bytes.Index(r.d[from:lim], deadcode)
	if j < 0 {
		fail("alte Klasse %s: Endmarke fehlt", name)
	}
	r.p = from + j + 4
	return ps
}

func (r *reader) vec3(v *[3]float32) {
	v[0], v[1], v[2] = r.f32(), r.f32(), r.f32()
}

// boxesSpheres liest worldTreeBoundary, localNodeBoundary, worldNodeBoundary
// und ueberspringt die beiden Kugeln.
func (r *reader) boxesSpheres(e *Entity) {
	r.vec3(&e.TreeMin)
	r.vec3(&e.TreeMax)
	r.skip(24)
	r.vec3(&e.NodeMin)
	r.vec3(&e.NodeMax)
	r.skip(16 * 2)
}

// entityCommon liest den eCEntity-Teil (Version 0x53).
func (r *reader) entityCommon(e *Entity) []PropSet {
	if v := r.u16(); v != 0x53 {
		fail("eCEntity-Version 0x%X nicht unterstuetzt", v)
	}
	r.skip(2)
	e.Span.GUID = r.p
	copy(e.GUID[:], r.bytes(20))
	r.skip(7)
	r.f32()
	r.skip(2 + 1 + 3)
	e.Name = r.entry()
	e.Span.Matrix = r.p
	r.floats(e.World[:])
	r.skip(64)
	r.boxesSpheres(e)
	r.f32()
	r.u8()
	r.f32()
	r.u32()
	r.f32()
	r.skip(2)
	n := int(r.i32())
	if n < 0 || n > 1000 {
		fail("unplausible Klassenzahl %d", n)
	}
	sets := make([]PropSet, 0, n)
	for i := 0; i < n; i++ {
		sets = append(sets, r.readClassNew())
	}
	return sets
}

func (r *reader) readLinks() {
	for {
		a := r.i32()
		r.i32()
		if a == -1 {
			return
		}
	}
}

// ParseWorldFile liest eine .lrentdat- oder .node-Datei in allen
// bekannten Formatversionen.
func ParseWorldFile(d []byte, source string) (ents []*Entity, err error) {
	defer recoverParse(&err)
	if len(d) < 16 {
		return nil, nil
	}
	var r *reader
	switch {
	case string(d[:8]) == "GENOMFLE":
		r, _ = genome(d)
		if len(d) >= 22 && string(d[14:22]) == "GENOMEDL" {
			r.p = 22
			if v := r.u16(); v != 0x53 {
				fail("lrentdat-Version 0x%X nicht unterstuetzt", v)
			}
			// eCEntityDynamicContext: Eigenschaften, Version, 33 Byte Zusatzdaten,
			// danach folgen die Entities.
			r.skip(6)
			r.entry()
			r.skip(5)
			var ps PropSet
			r.u16()
			r.u32()
			r.u16()
			n := int(r.u32())
			for i := 0; i < n; i++ {
				var p Property
				p.Name = r.entry()
				p.Type = r.entry()
				r.u16()
				p.Data = r.bytes(int(r.u32()))
				ps.Props = append(ps.Props, p)
			}
			r.u16()
			r.skip(33)
			return r.entities(source, fmtLrentdat)
		}
		if v := r.u16(); v != 0x53 {
			fail("node-Version 0x%X nicht unterstuetzt", v)
		}
		return r.entities(source, fmtNode)
	case d[0] == 0x48 && d[1] == 0:
		r = &reader{d: d}
		r.skip(4)
		var table []string
		if r.u8() != 0 {
			n := int(r.u32())
			if n > len(d) {
				fail("Stringtabelle defekt")
			}
			for i := 0; i < n; i++ {
				table = append(table, string(r.bytes(int(r.u16()))))
			}
		}
		if table == nil {
			table = []string{}
		}
		r.table = table
		r.u32()
		return r.entities(source, fmtOld48)
	case d[0] == 0x1e && d[1] == 0:
		r = &reader{d: d, p: 2}
		return r.entities(source, fmtOld1e)
	}
	return nil, fmt.Errorf("unbekanntes Dateiformat")
}

const (
	fmtLrentdat = iota
	fmtNode
	fmtOld48
	fmtOld1e
)

func (r *reader) entities(source string, format int) ([]*Entity, error) {
	n := int(r.i32())
	if n < 0 || n > 10000000 {
		fail("unplausible Entity-Anzahl %d", n)
	}
	ents := make([]*Entity, 0, n)
	r.countOff = r.p - 4
	for i := 0; i < n; i++ {
		e := &Entity{Source: source, Index: i}
		e.Span.Start = r.p
		e.Span.Vis = -1
		var sets []PropSet
		switch format {
		case fmtLrentdat:
			r.u16() // gCEntity 0x40
			r.u16() // eCDynamicEntity 0x53
			if r.u8() != 0 {
				r.skip(20)
			}
			sets = r.entityCommon(e)
		case fmtNode:
			hc := r.u8()
			dp := r.u8()
			r.u16() // eCSpatialEntity 0x23
			if r.u8() != 0 {
				r.skip(20)
			}
			e.Span.Vis = r.p
			r.skip(24 + 60)
			sets = r.entityCommon(e)
			if hc != 0 && dp == 0 {
				r.skip(20)
			}
		case fmtOld48, fmtOld1e:
			hc := r.u8()
			dp := r.u8()
			sv := r.u16()
			if r.u8() != 0 {
				r.skip(20)
			}
			if format == fmtOld48 {
				if sv != 0x23 {
					fail("alte Spatial-Version 0x%X", sv)
				}
				r.skip(24 + 60)
			}
			ev := r.u16()
			r.skip(2)
			copy(e.GUID[:], r.bytes(20))
			if format == fmtOld48 {
				if ev != 0x3f {
					fail("alte Entity-Version 0x%X", ev)
				}
				r.skip(8)
				r.f32()
				r.skip(6)
			} else {
				if ev != 0x2a {
					fail("alte Entity-Version 0x%X", ev)
				}
				r.skip(9)
				r.f32()
				r.skip(5)
			}
			e.Name = r.entry()
			r.floats(e.World[:])
			r.skip(64)
			r.boxesSpheres(e)
			r.f32()
			r.u8()
			r.f32()
			if format == fmtOld48 {
				r.u32()
				r.f32()
				r.skip(2)
			}
			nc := int(r.i32())
			if nc < 0 || nc > 1000 {
				fail("unplausible Klassenzahl %d", nc)
			}
			for j := 0; j < nc; j++ {
				sets = append(sets, r.readClassOld())
			}
			if hc != 0 && dp == 0 {
				r.skip(20)
			}
		}
		r.classify(e, sets)
		e.Span.End = r.p
		e.Span.Valid = format == fmtLrentdat || format == fmtNode
		ents = append(ents, e)
	}
	r.linksStart = r.p
	for {
		a := r.i32()
		b := r.i32()
		if a == -1 {
			break
		}
		r.links = append(r.links, [2]int32{a, b})
	}
	r.linksEnd = r.p
	return ents, nil
}

func (r *reader) classify(e *Entity, sets []PropSet) {
	has := map[string]*PropSet{}
	for i := range sets {
		e.Classes = append(e.Classes, sets[i].Class)
		has[sets[i].Class] = &sets[i]
	}
	meshOf := func(ps *PropSet, names ...string) string {
		for _, n := range names {
			if s := r.stringValue(ps.Get(n)); s != "" {
				return s
			}
		}
		return ""
	}
	if ps := has["eCVisualMeshStatic_PS"]; ps != nil {
		e.Mesh = meshOf(ps, "ResourceFileName", "ResourceFilePath")
		e.MatSwitch = intValue(ps.Get("MaterialSwitch"))
	} else if ps := has["eCVisualMeshDynamic_PS"]; ps != nil {
		e.Mesh = meshOf(ps, "ResourceFileName", "ResourceFilePath")
		e.MatSwitch = intValue(ps.Get("MaterialSwitch"))
	} else if ps := has["eCVisualAnimation_PS"]; ps != nil {
		e.Mesh = meshOf(ps, "ResourceFilePath", "ResourceFileName")
		e.MatSwitch = intValue(ps.Get("MaterialSwitch"))
	} else if ps := has["eCSpeedTree_PS"]; ps != nil {
		e.Mesh = meshOf(ps, "ResourceFilePath", "ResourceFileName")
	}
	low := strings.ToLower(e.Mesh)
	switch {
	case has["gCNPC_PS"] != nil:
		e.Kind = KindNPC
	case strings.HasSuffix(low, ".xcmsh") || strings.HasSuffix(low, ".xlmsh"):
		e.Kind = KindMesh
	case strings.HasSuffix(low, ".fxa") || strings.HasSuffix(low, ".xact"):
		e.Kind = KindAnimated
	case has["eCSpeedTree_PS"] != nil:
		e.Kind = KindTree
	case has["eCStaticPointLight_PS"] != nil || has["eCPointLight_PS"] != nil:
		e.Kind = KindLight
	case has["gCItem_PS"] != nil:
		e.Kind = KindItem
	default:
		e.Kind = KindOther
	}
}
