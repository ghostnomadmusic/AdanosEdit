package g3

// Vorlagen (.tple aus Templates.pak) lesen und daraus neue Welt-Entities bauen.
//
// Eine Vorlage besteht aus einem "Item-Header" (Wurzel) und dem eigentlichen
// "Referenz-Header" mit den Property-Sets. Eine im Spiel platzierte Entity
// verweist ueber ihre Creator-GUID auf den Referenz-Header und enthaelt eine
// Kopie seiner Property-Sets (siehe eCEntity::CreateFromTemplate).
//
// Strings sind in Genome-Dateien Indizes in eine Stringtabelle. Beim Uebertragen
// in eine Weltdatei muessen deshalb alle String-Stellen der Property-Sets
// gefunden und auf die Tabelle der Zieldatei umgeschrieben werden. Das
// erledigt der Scanner unten; Vorlagen mit unbekannten Strukturen werden als
// nicht platzierbar markiert.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

// TplClass ist ein Property-Set einer Vorlage im Welt-Format (ohne die
// Vorlagen-Vorsatzdaten): Version, Unterklassen-Kopf, Inhalt.
type TplClass struct {
	Name    string
	Version uint16
	Body    []byte // ab 01 00 01 01 00 01 bis vor DEADC0DE
	Entries []int  // Positionen der String-Indizes in Body
}

// Template ist eine platzierbare Vorlage.
type Template struct {
	Path     string // Pfad im Archiv
	Name     string // Entity-Name (z. B. It_Potion_Health_01)
	File     string
	RefGUID  [20]byte
	Stamp    uint32
	Flags    TplFlags
	LocalMin [3]float32
	LocalMax [3]float32
	Classes  []TplClass
	Table    []string
	Mesh     string     // Anzeige-Modell (xcmsh, fxa, spt)
	Kind     EntityKind // KindMesh, KindItem ...
	Category string     // Items, Objects, NPC ...
	Problem  string     // nicht leer: nicht platzierbar (Grund)
	Children int
}

// TplFlags sind die privaten Entity-Daten, die von der Vorlage kopiert werden.
type TplFlags struct {
	Enabled, RenderingEnabled, ProcessingDisabled, DeactivationEnabled bool
	Pickable, CollisionEnabled, UnkFlag2                               bool
	RenderAlpha                                                        float32
	InsertType                                                         uint16
	LastRenderPriority                                                 byte
	VisualLoD                                                          float32
	ObjectCull                                                         float32
	SpecialDepthTex                                                    bool
	UniformScaling                                                     float32
	RangedCulling, OutFading                                           bool
}

// ---------------------------------------------------------------------------
// Scanner fuer String-Stellen

type entryScan struct {
	d     []byte
	p     int
	table []string
	offs  []int
}

func (s *entryScan) need(n int) {
	if n < 0 || s.p+n > len(s.d) {
		fail("Daten enden unerwartet (%d+%d)", s.p, n)
	}
}
func (s *entryScan) skip(n int) { s.need(n); s.p += n }
func (s *entryScan) u8() byte   { s.need(1); v := s.d[s.p]; s.p++; return v }
func (s *entryScan) u16() uint16 {
	s.need(2)
	v := binary.LittleEndian.Uint16(s.d[s.p:])
	s.p += 2
	return v
}
func (s *entryScan) u32() uint32 {
	s.need(4)
	v := binary.LittleEndian.Uint32(s.d[s.p:])
	s.p += 4
	return v
}
func (s *entryScan) entry() string {
	s.need(2)
	i := int(binary.LittleEndian.Uint16(s.d[s.p:]))
	if i >= len(s.table) {
		fail("String-Index %d ausserhalb der Tabelle (%d)", i, len(s.table))
	}
	s.offs = append(s.offs, s.p)
	s.p += 2
	return s.table[i]
}

var subClassID = []byte{1, 0, 1, 1, 0, 1}

// subclass: 01 00 01 01 00 01, Name, 01 00 00 53 00, Inhalt
func (s *entryScan) subclass() string {
	s.need(6)
	if !bytes.Equal(s.d[s.p:s.p+6], subClassID) {
		fail("Unterklassen-Kennung fehlt bei %d", s.p)
	}
	s.p += 6
	name := s.entry()
	s.skip(5)
	s.body(name)
	return name
}

var stringTypes = map[string]bool{
	"bCAnimationResourceString": true, "bCImageOrMaterialResourceString": true, "bCImageResourceString": true,
	"bCMeshResourceString": true, "bCScriptString": true, "bCSpeedTreeResourceString": true,
	"bCString": true, "eCLocString": true,
}

// Typen, die keine Strings enthalten (Praefixe genuegen).
var plainTypePrefixes = []string{
	"bool", "char", "float", "int", "long", "short", "unsigned", "bCBox", "bCEulerAngles", "bCFloatAlphaColor",
	"bCFloatColor", "bCGuid", "bCMatrix", "bCPropertyID", "bCRange", "bCRect", "bCVector", "bCQuaternion",
	"bCMotion", "bCOrientedBox", "bCSphere", "bCDateTime",
	"bTPropertyContainer<", "bTObjArray<class bTPropertyContainer<", "bTValArray<", "bTObjArray<class eCEntityProxy>",
	"bTObjArray<class eCTemplateEntityProxy>", "eCEntityProxy", "eCTemplateEntityProxy", "gCSkillValue",
	"bTSceneObjArray<", "eCScriptProxyScript", "eCTemplateEntityProxy",
}

// UnknownTypes sammelt (fuer Tests) unbekannte Property-Typen.
var UnknownTypes = map[string]int{}

func (s *entryScan) prop(typ string, end int) {
	switch {
	case stringTypes[typ]:
		s.entry()
	case typ == "bTObjArray<class bCString>":
		s.skip(1)
		n := int(s.u32())
		for i := 0; i < n; i++ {
			s.entry()
		}
	case strings.HasPrefix(typ, "bTPropertyObject<class "):
		cls := strings.TrimPrefix(typ, "bTPropertyObject<class ")
		if i := strings.IndexAny(cls, ",>"); i >= 0 {
			cls = strings.TrimSpace(cls[:i])
		}
		s.body(cls)
	case typ == "eCPropertySetProxy":
		s.u16()
		if s.u8() != 0 {
			s.entry()
		}
		s.p = end
	default:
		for _, pre := range plainTypePrefixes {
			if strings.HasPrefix(typ, pre) {
				s.p = end
				return
			}
		}
		UnknownTypes[typ]++
		fail("unbekannter Eigenschaftstyp %s", typ)
	}
	if s.p != end {
		fail("Eigenschaft vom Typ %s: Laenge passt nicht", typ)
	}
}

// Klassen, deren Zusatzdaten (nach der Klassenversion) keine Strings enthalten.
var plainPostClasses = map[string]bool{
	"eCRigidBody_PS": true, "eCIlluminated_PS": true, "gCAnchor_PS": true, "gCEnclave_PS": true, "gCParty_PS": true,
	"eCVisualMeshDynamic_PS": true, "eCVisualMeshStatic_PS": true, "eCSpeedTree_PS": true, "eCSpeedTreeWind_PS": true,
	"gCProjectile_PS": true, "eCParticle_PS": true, "gCInventorySlot": true, "gCInventoryStack": true,
	"eCMesh_PS": true, "eCPointLight_PS": true, "eCSpotLight_PS": true, "eCDirectionalLight_PS": true,
	"eCAudioEmitter_PS": true, "eCAudioReverbZone_PS": true, "gCLock_PS": true, "gCInteraction_PS": true,
	"gCDoor_PS": true, "gCRecipe_PS": true, "gCLetter_PS": true, "gCBook_PS": true, "gCDamage_PS": true,
	"gCTreasureSet_PS": true, "gCCollisionCircle_PS": true, "gCNavOffset_PS": true, "gCPrefPath_PS": true,
	"eCDecal_PS": true, "eCSpringAndDamperEffector": true, "eCStrip_PS": true, "eCWeatherZone_PS": true, "gCFocus_PS": true,
}

func (s *entryScan) body(name string) {
	ver := s.u16()
	size := int(s.u32())
	end := s.p + size
	if end > len(s.d) || size < 0 {
		fail("Klasse %s: ungueltige Groesse", name)
	}
	if ver == 1 {
		s.entry()
	}
	if ver <= 0x51 {
		s.skip(20)
	}
	s.u16()
	n := int(s.u32())
	if n > 100000 {
		fail("Klasse %s: unplausible Eigenschaftszahl", name)
	}
	props := map[string][]byte{}
	for i := 0; i < n; i++ {
		pn := s.entry()
		pt := s.entry()
		s.u16()
		sz := int(s.u32())
		st := s.p
		s.need(sz)
		props[pn] = s.d[st : st+sz]
		s.prop(pt, st+sz)
	}
	cv := s.u16()
	s.post(name, cv, end, props)
	if s.p != end {
		fail("Klasse %s: Zusatzdaten nicht verstanden (%d Byte)", name, end-s.p)
	}
}

func enumVal(b []byte) int {
	if len(b) < 6 {
		return -1
	}
	return int(int32(binary.LittleEndian.Uint32(b[2:])))
}

func (s *entryScan) post(name string, cv uint16, end int, props map[string][]byte) {
	if s.p == end {
		return
	}
	switch name {
	case "eCCollisionShape_PS":
		n := int(s.u32())
		for i := 0; i < n; i++ {
			s.subclass()
		}
	case "eCCollisionShape":
		t := enumVal(props["ShapeType"])
		if t == 1 || t == 6 {
			s.entry()
		}
		s.p = end
	case "gCInventory_PS":
		s.u16()
		n := int(s.u32())
		for i := 0; i < n; i++ {
			s.subclass()
		}
		s.u16()
		slots := int(s.u32())
		for i := 0; i < slots; i++ {
			s.need(3)
			if s.d[s.p+2] == 1 {
				s.subclass()
			} else {
				s.skip(3)
			}
		}
		s.p = end // 5 eCEntityProxy
	case "gCItem_PS":
		s.skip(24)
		s.u32()
		s.u8()
		s.skip(1)
		n := int(s.u32())
		for i := 0; i < n; i++ {
			s.u16()
			for k := 0; k < 5; k++ {
				s.entry()
			}
		}
		s.u8()
	case "gCMap_PS":
		cnt := int(s.u32())
		det := int(s.u32())
		if det == 0x14 {
			for i := 0; i < cnt; i++ {
				s.entry()
				s.skip(13)
			}
		} else {
			s.skip(cnt * det)
		}
	case "eCVisualAnimation_PS":
		s.skip(2)
		slot := func(named bool) {
			if named {
				s.entry()
			}
			s.skip(2)
			s.entry()
			s.u32()
			if s.u8() != 0 {
				s.skip(2)
				s.entry()
				s.u32()
			}
		}
		slot(false)
		n := int(s.u32())
		for i := 0; i < n; i++ {
			slot(true)
			s.u8()
		}
		s.u8()
		n = int(s.u32())
		for i := 0; i < n; i++ {
			s.entry()
		}
		n = int(s.u32())
		for i := 0; i < n; i++ {
			s.skip(2)
			if s.u8() != 0 {
				s.skip(20)
			}
			s.entry()
			s.skip(72)
		}
		s.skip(24)
		s.p = end // eCEntityPropertySet: Version + Flag
	default:
		if plainPostClasses[name] || end-s.p <= 3 {
			s.p = end
			return
		}
		fail("Zusatzdaten der Klasse %s (%d Byte) unbekannt", name, end-s.p)
	}
}

// ScanClassEntries findet die String-Stellen eines Property-Sets im
// Welt-Format (ab Unterklassen-Kopf). Liefert Offsets relativ zu b.
func ScanClassEntries(b []byte, table []string) (offs []int, err error) {
	defer recoverParse(&err)
	s := &entryScan{d: b, table: table}
	s.subclass()
	if s.p != len(b) {
		fail("Klassenende passt nicht")
	}
	return s.offs, nil
}

// ---------------------------------------------------------------------------
// Vorlagen lesen

// ParseTemplate liest eine .tple-Datei.
func ParseTemplate(d []byte, path string) (t *Template, err error) {
	defer recoverParse(&err)
	r, ok := genome(d)
	if !ok {
		return nil, fmt.Errorf("keine Genome-Datei")
	}
	r.p = 14
	if string(r.bytes(8)) != "GENOMETP" {
		fail("keine Vorlage")
	}
	r.skip(4)
	if r.u8() != 0 {
		n := int(r.u32())
		idx := make([]string, n)
		for i := 0; i < n; i++ {
			k := int(r.u16())
			if k >= len(r.table) {
				fail("Index-Tabelle defekt")
			}
			idx[i] = r.table[k]
		}
		r.table = idx
	}
	r.u32() // Groesse des Codeabschnitts
	n := int(r.u32())
	if n < 1 || n > 10000 {
		fail("unplausible Kopfzahl %d", n)
	}
	type hdr struct {
		file, name   string
		guid         [20]byte
		helper       bool
		ref          bool
		flags        TplFlags
		lmn, lmx     [3]float32
		stamp        uint32
		classes      []TplClass
		classStart   int
		deleted      bool
		classProblem string
	}
	hs := make([]hdr, n)
	for i := 0; i < n; i++ {
		h := &hs[i]
		hv := r.u16()
		h.file = r.entry()
		copy(h.guid[:], r.bytes(20))
		f := &h.flags
		f.Enabled = r.u8() != 0
		f.RenderingEnabled = r.u8() != 0
		f.ProcessingDisabled = r.u8() != 0
		f.DeactivationEnabled = r.u8() != 0
		f.Pickable = r.u8() != 0
		f.CollisionEnabled = r.u8() != 0
		f.UnkFlag2 = r.u8() != 0
		h.helper = r.u8() != 0
		if r.u8() != 0 {
			h.ref = true
			r.skip(20)
		}
		f.RenderAlpha = r.f32()
		f.InsertType = r.u16()
		f.LastRenderPriority = r.u8()
		h.name = r.entry()
		r.skip(128) // Welt- und lokale Matrix
		r.skip(24)
		r.vec3(&h.lmn)
		r.vec3(&h.lmx)
		r.skip(24)
		f.VisualLoD = r.f32()
		r.u8() // unkFlag3
		h.deleted = r.u8() != 0
		f.ObjectCull = 1
		if hv >= 40 {
			f.ObjectCull = r.f32()
		}
		if hv >= 56 {
			h.stamp = r.u32()
		}
		if hv >= 58 {
			f.SpecialDepthTex = r.u8() != 0
		}
		f.UniformScaling = 1
		if hv >= 61 {
			f.UniformScaling = r.f32()
		}
		if hv >= 62 {
			f.RangedCulling = r.u8() != 0
			f.OutFading = r.u8() != 0
		}
		if !h.ref {
			nc := int(r.u32())
			if nc < 0 || nc > 1000 {
				fail("unplausible Klassenzahl")
			}
			for k := 0; k < nc; k++ {
				ver := r.u16()
				pre := int(r.u32())
				st := r.p
				r.entry() // Typname
				bstart := r.p
				r.need(pre)
				// Ende: DEADC0DE nach pre Bytes (Groesse zaehlt bis DEADC0DE-2)
				end := st + pre - 2
				if end < bstart || end+4 > len(r.d) || binary.LittleEndian.Uint32(r.d[end:]) != 0xDEADC0DE {
					j := bytes.Index(r.d[bstart:], deadcode)
					if j < 0 {
						fail("Klassenende nicht gefunden")
					}
					end = bstart + j
				}
				body := r.d[bstart:end]
				c := TplClass{Version: ver, Body: body}
				offs, e := ScanClassEntries(body, r.table)
				if e != nil && h.classProblem == "" {
					h.classProblem = e.Error()
				}
				c.Entries = offs
				if len(body) > 8 {
					i := int(binary.LittleEndian.Uint16(body[6:]))
					if i < len(r.table) {
						c.Name = r.table[i]
					}
				}
				h.classes = append(h.classes, c)
				r.p = end + 4
			}
		}
	}
	children := map[int][]int{}
	for {
		a := r.i32()
		b := r.i32()
		if a == -1 {
			break
		}
		children[int(a)] = append(children[int(a)], int(b))
	}
	// Referenz-Header: erster Header ohne Helfer-Flag
	ri := -1
	for i := range hs {
		if !hs[i].helper {
			ri = i
			break
		}
	}
	if ri < 0 {
		ri = 0
	}
	h := hs[ri]
	t = &Template{Path: path, Name: h.name, File: h.file, RefGUID: h.guid, Stamp: h.stamp, Flags: h.flags,
		LocalMin: h.lmn, LocalMax: h.lmx, Classes: h.classes, Table: r.table, Children: len(children[ri])}
	if i := strings.Index(path, "/"); i > 0 {
		t.Category = path[:i]
	}
	switch {
	case h.deleted:
		t.Problem = "Vorlage ist als gelöscht markiert"
	case h.ref:
		t.Problem = "Objektgruppe (verweist auf andere Vorlage)"
	case t.Children > 0:
		t.Problem = "besteht aus mehreren Teilen (z. B. NPC)"
	case h.classProblem != "":
		t.Problem = h.classProblem
	}
	// Anzeige-Modell bestimmen (wie bei Welt-Entities)
	var sets []PropSet
	for _, c := range h.classes {
		rr := &reader{d: c.Body, table: r.table}
		func() {
			defer func() { recover() }()
			rr.skip(6)
			name := rr.entry()
			rr.skip(5)
			ps := PropSet{Class: name}
			rr.propsBody(&ps)
			sets = append(sets, ps)
		}()
	}
	e := &Entity{Name: h.name}
	r.classify(e, sets)
	t.Mesh = e.Mesh
	t.Kind = e.Kind
	return t, nil
}

// ---------------------------------------------------------------------------
// Neue Entity in eine Weltdatei einfuegen

// AddFromTemplate fuegt der (lrentdat-)Datei eine neue Entity aus der Vorlage
// an der Weltmatrix world hinzu und liefert ihren Index.
func (wf *WorldFile) AddFromTemplate(t *Template, world [16]float32) (int, error) {
	if !wf.IsLrent {
		return -1, fmt.Errorf("neue Objekte koennen nur in .lrentdat-Dateien angelegt werden")
	}
	if t.Problem != "" {
		return -1, fmt.Errorf("%s", t.Problem)
	}
	var b bytes.Buffer
	w16 := func(v uint16) { binary.Write(&b, binary.LittleEndian, v) }
	w32 := func(v uint32) { binary.Write(&b, binary.LittleEndian, v) }
	wf32 := func(v float32) { binary.Write(&b, binary.LittleEndian, v) }
	wb := func(v bool) {
		if v {
			b.WriteByte(1)
		} else {
			b.WriteByte(0)
		}
	}
	f := t.Flags
	w16(0x40)
	w16(0x53)
	wb(true)
	b.Write(t.RefGUID[:])
	start := 0
	w16(0x53)
	w16(1)
	guidOff := b.Len()
	g := NewGUID()
	b.Write(g[:])
	wb(f.Enabled)
	wb(f.RenderingEnabled)
	wb(f.ProcessingDisabled)
	wb(f.DeactivationEnabled)
	wb(false) // unkFlag1
	wb(f.Pickable)
	wb(f.CollisionEnabled)
	wf32(f.RenderAlpha)
	w16(f.InsertType)
	b.WriteByte(f.LastRenderPriority)
	wb(false) // locked
	wb(f.SpecialDepthTex)
	wb(f.UnkFlag2)
	w16(uint16(wf.stringIndex(t.Name)))
	matOff := b.Len()
	for _, v := range world {
		wf32(v)
	}
	for _, v := range world { // lokale Matrix (Elternteil = Wurzel, wird unten korrigiert)
		wf32(v)
	}
	box := func(mn, mx [3]float32) {
		for _, v := range mn {
			wf32(v)
		}
		for _, v := range mx {
			wf32(v)
		}
	}
	box(t.LocalMin, t.LocalMax) // Teilbaum (wird neu berechnet)
	box(t.LocalMin, t.LocalMax)
	box(t.LocalMin, t.LocalMax) // Knotenbox (wird neu berechnet)
	b.Write(make([]byte, 32))   // Kugeln
	wf32(f.VisualLoD)
	wb(true) // unkFlag3
	wf32(f.ObjectCull)
	w32(t.Stamp)
	wf32(f.UniformScaling)
	wb(f.RangedCulling)
	wb(f.OutFading)
	nc := 0
	for _, c := range t.Classes {
		if c.Name != "eCEditorEntityPropertySet" {
			nc++
		}
	}
	w32(uint32(nc))
	for _, c := range t.Classes {
		if c.Name == "eCEditorEntityPropertySet" {
			continue
		}
		w16(c.Version)
		body := append([]byte(nil), c.Body...)
		for _, o := range c.Entries {
			i := int(binary.LittleEndian.Uint16(body[o:]))
			binary.LittleEndian.PutUint16(body[o:], uint16(wf.stringIndex(t.Table[i])))
		}
		b.Write(body)
		w32(0xDEADC0DE)
	}
	raw := b.Bytes()
	e := &Entity{Name: t.Name, Mesh: t.Mesh, Kind: t.Kind, World: world, Index: len(wf.Ents), Source: wf.Path}
	for _, c := range t.Classes {
		e.Classes = append(e.Classes, c.Name)
	}
	copy(e.GUID[:], g[:16])
	e.Span = Span{Start: start, End: len(raw), GUID: guidOff, Matrix: matOff, Vis: -1, Valid: true}
	e.NodeMin, e.NodeMax = TransformBox(world, t.LocalMin, t.LocalMax)
	wf.Ents = append(wf.Ents, e)
	wf.Raw = append(wf.Raw, raw)
	wf.Deleted = append(wf.Deleted, false)
	wf.Links = append(wf.Links, [2]int32{0, int32(e.Index)})
	wf.SetWorld(e.Index, world)
	return e.Index, nil
}

// NewGUID erzeugt eine zufaellige GUID im Genome-Format (16 Byte + gueltig-Flag).
func NewGUID() [20]byte {
	var g [20]byte
	randRead(g[:16])
	g[7] = g[7]&0x0f | 0x40
	g[8] = g[8]&0x3f | 0x80
	return g
}

// stringIndex liefert den Index eines Strings in der Tabelle der Datei
// (neue Strings werden angehaengt).
func (wf *WorldFile) stringIndex(s string) int {
	if wf.strIdx == nil {
		wf.strIdx = map[string]int{}
		for i, x := range wf.Table {
			if _, ok := wf.strIdx[x]; !ok {
				wf.strIdx[x] = i
			}
		}
	}
	if i, ok := wf.strIdx[s]; ok {
		return i
	}
	wf.Table = append(wf.Table, s)
	wf.strIdx[s] = len(wf.Table) - 1
	wf.tableChanged = true
	return len(wf.Table) - 1
}

// DebugClasses liefert (fuer Tests) die Property-Sets einer lrentdat-Entity
// im Vorlagenformat sowie Creator-GUID und Zeitstempel.
func DebugClasses(wf *WorldFile, i int) (cls []TplClass, creator [20]byte, stamp uint32) {
	defer func() { recover() }()
	raw := wf.Raw[i]
	r := &reader{d: raw, table: wf.Table}
	r.skip(4)
	if r.u8() != 0 {
		copy(creator[:], r.bytes(20))
	}
	r.skip(4 + 20 + 7 + 4 + 2 + 1 + 3)
	r.entry()
	r.skip(128 + 72 + 32 + 4 + 1 + 4)
	stamp = r.u32()
	r.skip(4 + 2)
	n := int(r.u32())
	for k := 0; k < n; k++ {
		ver := r.u16()
		st := r.p
		r.skip(6)
		name := r.entry()
		r.skip(5)
		r.skip(2)
		size := int(r.u32())
		r.skip(size)
		cls = append(cls, TplClass{Name: name, Version: ver, Body: raw[st:r.p]})
		r.skip(4)
	}
	return
}
