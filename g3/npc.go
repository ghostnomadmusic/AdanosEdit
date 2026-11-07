package g3

// Neue NPCs: einen vorhandenen Welt-NPC samt Ausruestung (Kopf, Koerper ...)
// in eine andere Weltdatei kopieren, umbenennen und anpassen.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
)

// rawClass beschreibt die Lage eines Property-Sets in den Rohdaten einer Entity.
type rawClass struct {
	Name      string
	start     int // Klassenversion (u16)
	body, end int // Unterklassen-Kopf .. vor DEADC0DE
}

// layout liefert Namensposition und Property-Sets einer lrentdat-Entity.
func (wf *WorldFile) layout(i int) (nameOff int, classes []rawClass, err error) {
	defer recoverParse(&err)
	raw := wf.Raw[i]
	r := &reader{d: raw, table: wf.Table}
	r.skip(4)
	if r.u8() != 0 {
		r.skip(20)
	}
	r.skip(4 + 20 + 7 + 4 + 2 + 1 + 3)
	nameOff = r.p
	r.entry()
	r.skip(128 + 72 + 32 + 4 + 1 + 4 + 4 + 4 + 2)
	n := int(r.u32())
	for k := 0; k < n; k++ {
		st := r.p
		r.u16()
		b := r.p
		r.skip(6)
		name := r.entry()
		r.skip(5)
		r.u16()
		r.skip(int(r.u32()))
		classes = append(classes, rawClass{Name: name, start: st, body: b, end: r.p})
		if r.u32() != 0xDEADC0DE {
			fail("Klasse %s: Endmarke fehlt", name)
		}
	}
	return nameOff, classes, nil
}

// EntityClass liefert ein Property-Set einer Entity zum Bearbeiten.
func (wf *WorldFile) EntityClass(i int, name string) (*ClassBody, error) {
	_, cls, err := wf.layout(i)
	if err != nil {
		return nil, err
	}
	for _, c := range cls {
		if c.Name == name {
			return ParseClassBody(wf.Raw[i][c.body:c.end], wf.Table)
		}
	}
	return nil, fmt.Errorf("Property-Set %s fehlt", name)
}

// SetEntityClass ersetzt ein Property-Set einer Entity.
func (wf *WorldFile) SetEntityClass(i int, cb *ClassBody) error {
	_, cls, err := wf.layout(i)
	if err != nil {
		return err
	}
	for _, c := range cls {
		if c.Name == cb.Name {
			raw := wf.Raw[i]
			var out bytes.Buffer
			out.Write(raw[:c.body])
			out.Write(cb.Bytes())
			out.Write(raw[c.end:])
			wf.Raw[i] = out.Bytes()
			wf.mark(i)
			wf.Dirty = true
			return nil
		}
	}
	return fmt.Errorf("Property-Set %s fehlt", cb.Name)
}

// Intern liefert den Tabellenindex eines Strings (neu angelegt, falls noetig).
func (wf *WorldFile) Intern(s string) uint16 { return uint16(wf.stringIndex(s)) }

// StrData kodiert einen String-Eigenschaftswert fuer diese Datei.
func (wf *WorldFile) StrData(s string) []byte {
	return binary.LittleEndian.AppendUint16(nil, wf.Intern(s))
}

// StrValue liest einen String-Eigenschaftswert.
func (wf *WorldFile) StrValue(d []byte) string {
	if len(d) < 2 {
		return ""
	}
	i := int(binary.LittleEndian.Uint16(d))
	if i < len(wf.Table) {
		return wf.Table[i]
	}
	return ""
}

// SetName setzt den Entity-Namen.
func (wf *WorldFile) SetName(i int, name string) error {
	off, _, err := wf.layout(i)
	if err != nil {
		return err
	}
	binary.LittleEndian.PutUint16(wf.Raw[i][off:], wf.Intern(name))
	wf.Ents[i].Name = name
	wf.mark(i)
	wf.Dirty = true
	return nil
}

// Creator liefert die Vorlagen-GUID (Referenz-Header) einer lrentdat-Entity.
func (wf *WorldFile) Creator(i int) (g [16]byte, ok bool) {
	raw := wf.Raw[i]
	if len(raw) > 25 && raw[4] != 0 {
		copy(g[:], raw[5:21])
		return g, true
	}
	return g, false
}

// GUIDOf liefert die GUID einer Entity.
func (wf *WorldFile) GUIDOf(i int) [16]byte { return wf.Ents[i].GUID }

// copyRawTo kodiert eine Entity aus src fuer die Stringtabelle von dst um
// (Name und alle String-Stellen der Property-Sets) und gibt ihr eine neue GUID.
func (src *WorldFile) copyRawTo(i int, dst *WorldFile) ([]byte, [16]byte, error) {
	raw := append([]byte(nil), src.Raw[i]...)
	nameOff, cls, err := src.layout(i)
	if err != nil {
		return nil, [16]byte{}, err
	}
	remap := func(off int) {
		k := int(binary.LittleEndian.Uint16(raw[off:]))
		if k < len(src.Table) {
			binary.LittleEndian.PutUint16(raw[off:], dst.Intern(src.Table[k]))
		}
	}
	remap(nameOff)
	for _, c := range cls {
		offs, err := ScanClassEntries(raw[c.body:c.end], src.Table)
		if err != nil {
			return nil, [16]byte{}, fmt.Errorf("%s: %v", c.Name, err)
		}
		for _, o := range offs {
			remap(c.body + o)
		}
	}
	g := NewGUID()
	e := src.Ents[i]
	copy(raw[e.Span.GUID-e.Span.Start:], g[:16])
	var gg [16]byte
	copy(gg[:], g[:16])
	return raw, gg, nil
}

// CopyEntityTree kopiert eine Entity samt Kindern aus src nach dst (unter
// parent). Liefert die neuen Indizes (erst die Wurzel, dann die Kinder in
// gleicher Reihenfolge wie im Original) und eine Zuordnung alte->neue GUID.
func (dst *WorldFile) CopyEntityTree(src *WorldFile, i, parent int) ([]int, map[[16]byte][16]byte, error) {
	if !dst.IsLrent || !src.IsLrent {
		return nil, nil, fmt.Errorf("nur zwischen .lrentdat-Dateien moeglich")
	}
	guids := map[[16]byte][16]byte{}
	var out []int
	var walk func(k, par int) error
	walk = func(k, par int) error {
		raw, g, err := src.copyRawTo(k, dst)
		if err != nil {
			return err
		}
		e := *src.Ents[k]
		guids[e.GUID] = g
		e.GUID = g
		e.Index = len(dst.Ents)
		e.Source = dst.Path
		off := e.Span.Start
		e.Span.Start -= off
		e.Span.End -= off
		e.Span.GUID -= off
		e.Span.Matrix -= off
		e.Span.End = len(raw)
		ni := e.Index
		dst.Ents = append(dst.Ents, &e)
		dst.Raw = append(dst.Raw, raw)
		dst.Deleted = append(dst.Deleted, false)
		dst.Links = append(dst.Links, [2]int32{int32(par), int32(ni)})
		dst.mark(ni)
		out = append(out, ni)
		for _, c := range src.children(k) {
			if err := walk(c, ni); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(i, parent); err != nil {
		return nil, nil, err
	}
	dst.Dirty = true
	// Weltmatrix neu setzen (lokale Matrizen relativ zum neuen Elternteil)
	dst.SetWorld(out[0], dst.matrix(out[0], false))
	return out, guids, nil
}

// AddChildFromTemplate legt eine Entity aus einer Vorlage als Kind von parent
// an (gleiche Weltmatrix wie parent) und liefert ihren Index.
func (wf *WorldFile) AddChildFromTemplate(t *Template, parent int) (int, error) {
	world := wf.matrix(parent, false)
	i, err := wf.AddFromTemplate(t, world)
	if err != nil {
		return -1, err
	}
	// Verknuepfung von der Wurzel auf parent umhaengen
	for k := len(wf.Links) - 1; k >= 0; k-- {
		if int(wf.Links[k][1]) == i {
			wf.Links[k][0] = int32(parent)
			break
		}
	}
	wf.SetWorld(i, world)
	return i, nil
}

// TreeEntities liefert eine Entity und alle Nachfahren.
func (wf *WorldFile) TreeEntities(i int) []int {
	out := []int{i}
	for _, c := range wf.children(i) {
		out = append(out, wf.TreeEntities(c)...)
	}
	return out
}

// Children liefert die (nicht geloeschten) Kinder einer Entity.
func (wf *WorldFile) Children(i int) []int { return wf.children(i) }

// Ausruestungsplaetze (gESlot)
const (
	SlotBeard = 4
	SlotArmor = 9
	SlotHair  = 15
	SlotHead  = 16
	SlotBody  = 17
)

// NPCSpec beschreibt einen neuen NPC.
type NPCSpec struct {
	Name     string
	Src      *WorldFile // Datei des Vorbild-NPCs
	SrcIdx   int
	World    [16]float32
	Parts    map[int]*Template // Platz -> neue Vorlage (nil-Eintrag = Platz leeren)
	Treasure map[string]string // Schatzsets nach Art (TS_Weaponry ...), "" = entfernen
	Enclave  *[16]byte         // Enklave (nil = wie Vorbild, Null-GUID = keine)
	Align    *int              // politische Ausrichtung (nil = wie Vorbild)
	Items    *[]StackItem      // Gegenstaende im Inventar (nil = wie Vorbild)
	Points   [3]*Template      // Routinepunkte Arbeit, Freizeit, Schlaf (nil = wie Arbeit bzw. wie Vorbild)
	Trader   bool
	TradeSet string // Haendler-Schatzset (TS_Trader_...)
	Voice    string // "" = unveraendert
}

// itemSlotOf liest den Ausruestungsplatz eines Ausruestungs-Kindes.
func (wf *WorldFile) itemSlotOf(i int) int {
	cb, err := wf.EntityClass(i, "gCItem_PS")
	if err != nil || len(cb.Post) < 30 {
		return -1
	}
	return int(int32(binary.LittleEndian.Uint32(cb.Post[26:])))
}

// CreateNPC legt in dst einen neuen NPC nach spec an und liefert die Indizes
// aller neu angelegten Entities (NPC zuerst).
func (dst *WorldFile) CreateNPC(spec NPCSpec) ([]int, error) {
	idxs, guids, err := dst.CopyEntityTree(spec.Src, spec.SrcIdx, 0)
	if err != nil {
		return nil, err
	}
	// Verweise innerhalb des kopierten Baums auf die neuen GUIDs umbiegen
	for _, k := range idxs {
		raw := dst.Raw[k]
		for o, n := range guids {
			raw = bytes.ReplaceAll(raw, o[:], n[:])
		}
		dst.Raw[k] = raw
	}
	npc := idxs[0]
	npcGUID := dst.GUIDOf(npc)
	if err := dst.SetName(npc, spec.Name); err != nil {
		return nil, err
	}
	created := append([]int(nil), idxs...)
	// Aussehen
	if len(spec.Parts) > 0 {
		icb, err := dst.EntityClass(npc, "gCInventory_PS")
		if err != nil {
			return nil, err
		}
		inv, err := ParseInventory(icb.Post, dst.Table)
		if err != nil {
			return nil, err
		}
		vcb, err := dst.EntityClass(npc, "eCVisualAnimation_PS")
		if err != nil {
			return nil, err
		}
		va, err := ParseVisAnim(vcb.Post, dst.Table)
		if err != nil {
			return nil, err
		}
		for slot, t := range spec.Parts {
			sname := SlotName(slot)
			// Eintraege im Animationsmodell des NPCs entfernen
			for k := 0; k < len(va.Parts); k++ {
				if va.Parts[k].Name == sname {
					va.Parts = append(va.Parts[:k], va.Parts[k+1:]...)
					if k < len(va.partFlag) {
						va.partFlag = append(va.partFlag[:k], va.partFlag[k+1:]...)
					}
					k--
				}
			}
			for k := 0; k < len(va.Extras); k++ {
				if va.Extras[k].Name == sname {
					va.Extras = append(va.Extras[:k], va.Extras[k+1:]...)
					k--
				}
			}
			// bisheriges Teil entfernen
			for _, c := range dst.children(npc) {
				if dst.itemSlotOf(c) == slot {
					dst.Delete(c)
				}
			}
			if t == nil {
				if slot < len(inv.Slots) {
					inv.Slots[slot] = InvSlot{}
				}
				continue
			}
			ci, err := dst.AddChildFromTemplate(t, npc)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", t.Name, err)
			}
			created = append(created, ci)
			cg := dst.GUIDOf(ci)
			// Ausruestung wird vom NPC gezeichnet: selbst unsichtbar, nicht anwaehlbar
			ce := dst.Ents[ci]
			fo := ce.Span.GUID - ce.Span.Start + 20
			dst.Raw[ci][fo+1] = 0 // RenderingEnabled
			dst.Raw[ci][fo+2] = 1 // ProcessingDisabled
			dst.Raw[ci][fo+5] = 0 // Pickable
			// in das Animationsmodell des NPCs eintragen
			if m, ok := templateVisSlot(t); ok {
				m.Name = sname
				va.Parts = append(va.Parts, m)
				va.partFlag = append(va.partFlag, 1)
			} else {
				x := VAExtra{Ver: 1, HasGUID: true, Name: sname, Data: extraIdentity()}
				copy(x.GUID[:], cg[:])
				va.Extras = append(va.Extras, x)
			}
			if cb, err := dst.EntityClass(ci, "gCInteraction_PS"); err == nil {
				if p := cb.Get("Owner"); p != nil {
					p.Data = ProxyData(p.Data, npcGUID)
				}
				dst.SetEntityClass(ci, cb)
			}
			if cb, err := dst.EntityClass(ci, "gCItem_PS"); err == nil && len(cb.Post) >= 36 {
				binary.LittleEndian.PutUint32(cb.Post[26:], uint32(slot))
				cb.Post[30] = 0 // nicht als lose Beute sichtbar
				cb.Post[31] = 1
				binary.LittleEndian.PutUint32(cb.Post[32:], 0)
				dst.SetEntityClass(ci, cb)
			}
			var tg [16]byte
			copy(tg[:], t.RefGUID[:16])
			if err := inv.SetSlot(slot, tg, cg, int32(slot)); err != nil {
				return nil, err
			}
		}
		icb.Post = inv.Bytes()
		if err := dst.SetEntityClass(npc, icb); err != nil {
			return nil, err
		}
		vcb.Post = va.Bytes(dst.Intern)
		if err := dst.SetEntityClass(npc, vcb); err != nil {
			return nil, err
		}
	}
	// Inventar: Haendler, Waffen, Beute ... (Schatzsets) und Gegenstaende
	change := map[string]string{}
	for k, v := range spec.Treasure {
		change[k] = v
	}
	if spec.TradeSet != "" {
		change["TS_Trader"] = spec.TradeSet
	}
	if len(change) > 0 {
		if err := dst.SetTreasureSets(npc, change); err != nil {
			return nil, err
		}
	}
	if spec.Items != nil {
		if err := dst.SetInventoryItems(npc, *spec.Items); err != nil {
			return nil, err
		}
	}
	if spec.Enclave != nil || spec.Align != nil {
		if err := dst.SetFaction(npc, spec.Enclave, spec.Align); err != nil {
			return nil, err
		}
	}
	if spec.Voice != "" {
		if cb, err := dst.EntityClass(npc, "gCNPC_PS"); err == nil {
			if p := cb.Get("Voice"); p != nil {
				p.Data = dst.StrData(spec.Voice)
			}
			dst.SetEntityClass(npc, cb)
		}
	}
	// Position
	dst.SetWorld(npc, spec.World)
	// Routinepunkt
	if cb, err := dst.EntityClass(npc, "gCNavigation_PS"); err == nil {
		// Punkte fuer Arbeit, Freizeit und Schlaf; nebeneinander um den NPC
		var ids [3][16]byte
		offs := [3]float32{0, 250, -250}
		for k, t := range spec.Points {
			if t == nil {
				if k > 0 {
					ids[k] = ids[0]
				}
				continue
			}
			w := spec.World
			// Versatz entlang der eigenen x-Achse des NPCs
			w[12] += w[0] * offs[k]
			w[13] += w[1] * offs[k]
			w[14] += w[2] * offs[k]
			fi, err := dst.AddFromTemplate(t, w)
			if err != nil {
				return nil, fmt.Errorf("Routinepunkt: %v", err)
			}
			created = append(created, fi)
			ids[k] = dst.GUIDOf(fi)
		}
		names := [3]string{"Working", "Relaxing", "Sleeping"}
		for i := range cb.Props {
			p := &cb.Props[i]
			for k, n := range names {
				if ids[k] == ([16]byte{}) {
					continue
				}
				switch p.Name {
				case n + "Point":
					p.Data = IDData(ids[k])
				case n + "Points":
					p.Data = IDArrayData(ids[k])
				}
			}
			switch p.Name {
			case "LastUseableNavigationPosition", "StartPosition":
				p.Data = make([]byte, 12)
			case "LastUseableNavigationZoneID":
				p.Data = IDData([16]byte{})
			}
		}
		dst.SetEntityClass(npc, cb)
	}
	return created, nil
}

// SlotName liefert den Namen eines Ausruestungsplatzes (Slot_Head ...).
func SlotName(slot int) string {
	switch slot {
	case SlotHead:
		return "Slot_Head"
	case SlotBody:
		return "Slot_Body"
	case SlotBeard:
		return "Slot_Beard"
	case SlotHair:
		return "Slot_Hair"
	case SlotArmor:
		return "Slot_Armor"
	}
	return fmt.Sprintf("Slot_%d", slot)
}

// templateVisSlot liest das Animationsmodell (FXA) einer Vorlage.
func templateVisSlot(t *Template) (MatSlot, bool) {
	for _, c := range t.Classes {
		if c.Name != "eCVisualAnimation_PS" {
			continue
		}
		cb, err := ParseClassBody(c.Body, t.Table)
		if err != nil {
			return MatSlot{}, false
		}
		va, err := ParseVisAnim(cb.Post, t.Table)
		if err != nil || va.Main.FXA == "" {
			return MatSlot{}, false
		}
		return va.Main, true
	}
	return MatSlot{}, false
}

// extraIdentity: Anhang ohne Versatz (Einheitsmatrix).
func extraIdentity() []byte {
	b := make([]byte, 72)
	b[0] = 1
	for i := 0; i < 4; i++ {
		binary.LittleEndian.PutUint32(b[4+i*20:], 0x3f800000)
	}
	copy(b[68:], []byte{0x63, 0x78, 0xfb, 0xfe})
	return b
}

// RoutinePoints liest die Plaetze fuer Arbeit, Freizeit und Schlaf eines NPCs.
func (wf *WorldFile) RoutinePoints(npc int) (pts [3][16]byte, err error) {
	cb, err := wf.EntityClass(npc, "gCNavigation_PS")
	if err != nil {
		return pts, err
	}
	for _, p := range cb.Props {
		for k, n := range []string{"WorkingPoint", "RelaxingPoint", "SleepingPoint"} {
			if p.Name == n && len(p.Data) >= 16 {
				copy(pts[k][:], p.Data[:16])
			}
		}
	}
	return pts, nil
}

// Routine ist ein Tagesablauf eines NPCs (z. B. "Start" oder ein per
// SetRoutine umschaltbarer Ablauf) mit den Plaetzen Arbeit, Freizeit, Schlaf.
type Routine struct {
	Name   string
	Points [3][16]byte
	tail   [3][4]byte // Zusatzwert der Kennung im Spiel (bleibt erhalten, solange der Platz gleich ist)
	orig   [3][16]byte
}

func (r *Routine) idData(k int) []byte {
	if r.Points[k] == r.orig[k] {
		return append(append([]byte(nil), r.Points[k][:]...), r.tail[k][:]...)
	}
	return IDData(r.Points[k])
}

var routinePointNames = [3]string{"Working", "Relaxing", "Sleeping"}

func parseIDArray(d []byte) [][20]byte {
	if len(d) < 5 {
		return nil
	}
	n := int(binary.LittleEndian.Uint32(d[1:]))
	var out [][20]byte
	for i := 0; i < n && 5+i*20+20 <= len(d); i++ {
		var g [20]byte
		copy(g[:], d[5+i*20:])
		out = append(out, g)
	}
	return out
}

func (wf *WorldFile) parseStrArray(d []byte) []string {
	if len(d) < 5 {
		return nil
	}
	n := int(binary.LittleEndian.Uint32(d[1:]))
	var out []string
	for i := 0; i < n && 5+i*2+2 <= len(d); i++ {
		out = append(out, wf.StrValue(d[5+i*2:]))
	}
	return out
}

func (wf *WorldFile) strArrayData(list []string) []byte {
	out := []byte{1}
	out = binary.LittleEndian.AppendUint32(out, uint32(len(list)))
	for _, s := range list {
		out = binary.LittleEndian.AppendUint16(out, wf.Intern(s))
	}
	return out
}

// Routines liest alle Tagesablaeufe eines NPCs und den aktuellen.
func (wf *WorldFile) Routines(npc int) (list []Routine, cur string, err error) {
	cb, err := wf.EntityClass(npc, "gCNavigation_PS")
	if err != nil {
		return nil, "", err
	}
	var names []string
	var single [3][20]byte
	var arrays [3][][20]byte
	for _, p := range cb.Props {
		switch p.Name {
		case "Routine":
			cur = wf.StrValue(p.Data)
		case "RoutineNames":
			names = wf.parseStrArray(p.Data)
		}
		for k, n := range routinePointNames {
			if p.Name == n+"Point" && len(p.Data) >= 20 {
				copy(single[k][:], p.Data[:20])
			}
			if p.Name == n+"Points" {
				arrays[k] = parseIDArray(p.Data)
			}
		}
	}
	if cur == "" {
		cur = "Start"
	}
	if len(names) == 0 {
		names = []string{cur}
	}
	for i, n := range names {
		r := Routine{Name: n}
		for k := range r.Points {
			var v [20]byte
			switch {
			case i < len(arrays[k]):
				v = arrays[k][i]
			default:
				v = single[k]
			}
			if strings.EqualFold(n, cur) && [16]byte(v[:16]) != [16]byte(single[k][:16]) && [16]byte(single[k][:16]) != ([16]byte{}) {
				v = single[k]
				v[16], v[17], v[18], v[19] = 0, 0, 0, 0
			}
			copy(r.Points[k][:], v[:16])
			copy(r.tail[k][:], v[16:])
			r.orig[k] = r.Points[k]
		}
		list = append(list, r)
	}
	return list, cur, nil
}

// SetRoutine legt einen Tagesablauf an oder ersetzt ihn (gleicher Name).
// Ist es der aktuelle Ablauf, gelten die Plaetze sofort.
func (wf *WorldFile) SetRoutine(npc int, r Routine) error {
	list, cur, err := wf.Routines(npc)
	if err != nil {
		return err
	}
	found := false
	for i := range list {
		if strings.EqualFold(list[i].Name, r.Name) {
			list[i].Points = r.Points
			found = true
		}
	}
	if !found {
		r.orig = [3][16]byte{}
		list = append(list, r)
	}
	return wf.writeRoutines(npc, list, cur)
}

// RemoveRoutine loescht einen Tagesablauf. Der Ablauf beim Spielstart
// (aktueller Ablauf) kann nicht geloescht werden.
func (wf *WorldFile) RemoveRoutine(npc int, name string) error {
	list, cur, err := wf.Routines(npc)
	if err != nil {
		return err
	}
	if strings.EqualFold(name, cur) {
		return fmt.Errorf("Der Tagesablauf beim Spielstart kann nicht gelöscht werden.")
	}
	var keep []Routine
	for _, r := range list {
		if !strings.EqualFold(r.Name, name) {
			keep = append(keep, r)
		}
	}
	if len(keep) == len(list) {
		return fmt.Errorf("Tagesablauf %s fehlt", name)
	}
	return wf.writeRoutines(npc, keep, cur)
}

func (wf *WorldFile) writeRoutines(npc int, list []Routine, cur string) error {
	cb, err := wf.EntityClass(npc, "gCNavigation_PS")
	if err != nil {
		return err
	}
	var names []string
	var arrays [3][]byte
	var counts int
	var curPts [3][]byte
	for i := range list {
		x := &list[i]
		names = append(names, x.Name)
		counts++
		for k := range arrays {
			arrays[k] = append(arrays[k], x.idData(k)...)
		}
		if strings.EqualFold(x.Name, cur) {
			for k := range curPts {
				curPts[k] = x.idData(k)
			}
		}
	}
	if curPts[0] == nil {
		for k := range curPts {
			curPts[k] = list[0].idData(k)
		}
	}
	// Einzelwerte: unveraenderte Plaetze behalten ihre bisherigen Rohdaten
	for _, p := range cb.Props {
		for k, n := range routinePointNames {
			if p.Name == n+"Point" && len(p.Data) >= 20 && len(curPts[k]) >= 16 && bytes.Equal(p.Data[:16], curPts[k][:16]) {
				curPts[k] = append([]byte(nil), p.Data...)
			}
		}
	}
	hasNames := false
	for i := range cb.Props {
		p := &cb.Props[i]
		if p.Name == "RoutineNames" {
			p.Data = wf.strArrayData(names)
			hasNames = true
		}
		for k, n := range routinePointNames {
			switch p.Name {
			case n + "Point":
				p.Data = curPts[k]
			case n + "Points":
				p.Data = append(binary.LittleEndian.AppendUint32([]byte{1}, uint32(counts)), arrays[k]...)
			}
		}
	}
	if !hasNames {
		return fmt.Errorf("RoutineNames fehlt")
	}
	return wf.SetEntityClass(npc, cb)
}

// SetRoutinePoint setzt einen Platz des aktuellen Tagesablaufs (0 = Arbeit,
// 1 = Freizeit, 2 = Schlaf) auf das Objekt mit der GUID id.
func (wf *WorldFile) SetRoutinePoint(npc, k int, id [16]byte) error {
	list, cur, err := wf.Routines(npc)
	if err != nil {
		return err
	}
	for _, r := range list {
		if strings.EqualFold(r.Name, cur) {
			r.Points[k] = id
			return wf.SetRoutine(npc, r)
		}
	}
	return fmt.Errorf("Tagesablauf %s fehlt", cur)
}

// Arten von Schatzsets (Inventar eines NPCs)
var TreasureKinds = []string{"TS_Plunder", "TS_Weaponry", "TS_PickPocket", "TS_Trader", "TS_Refresh"}

func treasureKind(s string) string {
	for _, k := range TreasureKinds {
		if strings.HasPrefix(s, k+"_") {
			return k
		}
	}
	return ""
}

// TreasureSets liest die fuenf Schatzsets eines NPCs.
func (wf *WorldFile) TreasureSets(npc int) ([]string, error) {
	cb, err := wf.EntityClass(npc, "gCInventory_PS")
	if err != nil {
		return nil, err
	}
	var out []string
	for i := 1; i <= 5; i++ {
		v := ""
		if p := cb.Get(fmt.Sprintf("TreasureSet%d", i)); p != nil {
			v = wf.StrValue(p.Data)
		}
		out = append(out, v)
	}
	return out, nil
}

// SetTreasureSets aendert Schatzsets nach Art (Schluessel aus TreasureKinds,
// Wert "" = entfernen). Bei Haendlern wird bei Bedarf TS_Refresh_Money ergaenzt.
func (wf *WorldFile) SetTreasureSets(npc int, change map[string]string) error {
	cur, err := wf.TreasureSets(npc)
	if err != nil {
		return err
	}
	var keep []string
	for _, s := range cur {
		if s == "" {
			continue
		}
		k := treasureKind(s)
		if _, ok := change[k]; ok {
			continue
		}
		if k == "TS_Refresh" {
			if v, ok := change["TS_Trader"]; ok && v == "" {
				continue // kein Haendler mehr
			}
		}
		keep = append(keep, s)
	}
	for _, k := range TreasureKinds {
		if v := change[k]; v != "" {
			keep = append(keep, v)
		}
	}
	if v := change["TS_Trader"]; v != "" {
		has := false
		for _, s := range keep {
			if treasureKind(s) == "TS_Refresh" {
				has = true
			}
		}
		if !has {
			keep = append(keep, "TS_Refresh_Money")
		}
	}
	// feste Reihenfolge nach Art
	sort.SliceStable(keep, func(i, j int) bool { return kindOrder(keep[i]) < kindOrder(keep[j]) })
	if len(keep) > 5 {
		return fmt.Errorf("zu viele Schatzsets (%d, hoechstens 5)", len(keep))
	}
	cb, err := wf.EntityClass(npc, "gCInventory_PS")
	if err != nil {
		return err
	}
	for i := 1; i <= 5; i++ {
		v := ""
		if i-1 < len(keep) {
			v = keep[i-1]
		}
		if p := cb.Get(fmt.Sprintf("TreasureSet%d", i)); p != nil {
			p.Data = wf.StrData(v)
		}
	}
	for _, n := range []string{"GeneratedPlunder", "GeneratedTrade"} {
		if p := cb.Get(n); p != nil {
			p.Data = []byte{0}
		}
	}
	if err := wf.SetEntityClass(npc, cb); err != nil {
		return err
	}
	if v, ok := change["TS_Trader"]; ok {
		if dcb, err := wf.EntityClass(npc, "gCDialog_PS"); err == nil {
			if p := dcb.Get("TradeEnabled"); p != nil {
				if v != "" {
					p.Data = []byte{1}
				} else {
					p.Data = []byte{0}
				}
			}
			wf.SetEntityClass(npc, dcb)
		}
	}
	return nil
}

func kindOrder(s string) int {
	k := treasureKind(s)
	for i, x := range TreasureKinds {
		if x == k {
			return i
		}
	}
	return len(TreasureKinds)
}

// InventoryItems liest die Gegenstaende im Inventar eines NPCs.
func (wf *WorldFile) InventoryItems(npc int) ([]StackItem, error) {
	cb, err := wf.EntityClass(npc, "gCInventory_PS")
	if err != nil {
		return nil, err
	}
	inv, err := ParseInventory(cb.Post, wf.Table)
	if err != nil {
		return nil, err
	}
	return inv.Stacks(wf.Table)
}

// SetInventoryItems ersetzt die Gegenstaende im Inventar eines NPCs.
func (wf *WorldFile) SetInventoryItems(npc int, items []StackItem) error {
	cb, err := wf.EntityClass(npc, "gCInventory_PS")
	if err != nil {
		return err
	}
	inv, err := ParseInventory(cb.Post, wf.Table)
	if err != nil {
		return err
	}
	inv.SetStacks(items, wf.Intern)
	cb.Post = inv.Bytes()
	return wf.SetEntityClass(npc, cb)
}

// Faction liest Enklave und politische Ausrichtung eines NPCs.
func (wf *WorldFile) Faction(npc int) (enclave [16]byte, align int, err error) {
	cb, err := wf.EntityClass(npc, "gCNPC_PS")
	if err != nil {
		return
	}
	if p := cb.Get("Enclave"); p != nil && len(p.Data) >= 16 {
		copy(enclave[:], p.Data[:16])
	}
	if p := cb.Get("PoliticalAlignment"); p != nil && len(p.Data) >= 6 {
		align = int(binary.LittleEndian.Uint32(p.Data[2:]))
	}
	return
}

// SetFaction setzt Enklave und/oder politische Ausrichtung (nil = unveraendert).
func (wf *WorldFile) SetFaction(npc int, enclave *[16]byte, align *int) error {
	cb, err := wf.EntityClass(npc, "gCNPC_PS")
	if err != nil {
		return err
	}
	if enclave != nil {
		if p := cb.Get("Enclave"); p != nil {
			p.Data = IDData(*enclave)
		}
	}
	if align != nil {
		if p := cb.Get("PoliticalAlignment"); p != nil {
			p.Data = EnumData(p.Data, int32(*align))
		}
	}
	return wf.SetEntityClass(npc, cb)
}

// Teach liest, was ein NPC lehrt (Attribute wie STR, Faehigkeiten als
// Vorlagen-GUIDs) und ob Handel/Lehre schon offen sind (gCDialog_PS).
func (wf *WorldFile) Teach(npc int) (attrs []string, skills [][16]byte, trade, teach bool, err error) {
	cb, err := wf.EntityClass(npc, "gCNPC_PS")
	if err != nil {
		return
	}
	if p := cb.Get("TeachAttribs"); p != nil {
		attrs = wf.parseStrArray(p.Data)
	}
	if p := cb.Get("TeachSkills"); p != nil && len(p.Data) >= 5 {
		n := int(binary.LittleEndian.Uint32(p.Data[1:]))
		for k := 0; k < n && 5+k*23+23 <= len(p.Data); k++ {
			o := 5 + k*23
			if p.Data[o+2] == 1 {
				var g [16]byte
				copy(g[:], p.Data[o+3:o+19])
				skills = append(skills, g)
			}
		}
	}
	if dl, e := wf.EntityClass(npc, "gCDialog_PS"); e == nil {
		if p := dl.Get("TradeEnabled"); p != nil && len(p.Data) > 0 {
			trade = p.Data[0] != 0
		}
		if p := dl.Get("TeachEnabled"); p != nil && len(p.Data) > 0 {
			teach = p.Data[0] != 0
		}
	}
	return
}

// SetTeach setzt, was ein NPC lehrt, und ob Handel/Lehre von Anfang an offen sind.
func (wf *WorldFile) SetTeach(npc int, attrs []string, skills [][16]byte, trade, teach bool) error {
	cb, err := wf.EntityClass(npc, "gCNPC_PS")
	if err != nil {
		return err
	}
	pa, ps := cb.Get("TeachAttribs"), cb.Get("TeachSkills")
	if pa == nil || ps == nil {
		return fmt.Errorf("TeachAttribs/TeachSkills fehlt")
	}
	pa.Data = wf.strArrayData(attrs)
	d := binary.LittleEndian.AppendUint32([]byte{1}, uint32(len(skills)))
	for _, g := range skills {
		d = append(d, 1, 0, 1)
		d = append(d, g[:]...)
		d = append(d, 0, 0, 0, 0)
	}
	ps.Data = d
	if err := wf.SetEntityClass(npc, cb); err != nil {
		return err
	}
	dl, err := wf.EntityClass(npc, "gCDialog_PS")
	if err != nil {
		return err
	}
	b := func(v bool) []byte {
		if v {
			return []byte{1}
		}
		return []byte{0}
	}
	if p := dl.Get("TradeEnabled"); p != nil {
		p.Data = b(trade)
	}
	if p := dl.Get("TeachEnabled"); p != nil {
		p.Data = b(teach)
	}
	return wf.SetEntityClass(npc, dl)
}

// TreasureKind liefert die Art eines Schatzsets (z. B. TS_Trader).
func TreasureKind(s string) string { return treasureKind(s) }
