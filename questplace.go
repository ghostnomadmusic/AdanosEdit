//go:build windows

package main

// Orte fuer Quests: Arenen, Zonen (Ort erreichen) und Truhen (ausrauben).

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"g3viewer/g3"
	"g3viewer/scene"
)

// findInstWithClass sucht ein (nicht geloeschtes) Objekt mit Namen und Property-Set.
func (a *app) findInstWithClass(name, cls string) int32 {
	if a.world == nil {
		return -1
	}
	for i := range a.world.Inst {
		in := &a.world.Inst[i]
		if in.Hidden || !strings.EqualFold(in.Ent.Name, name) {
			continue
		}
		if cls == "" || hasClass(in.Ent, cls) {
			return int32(i)
		}
	}
	return -1
}

// zoneItems liefert die Namen aller Zonen (bzw. nur Arenen) fuer die Vorschlagsliste.
func (a *app) zoneItems() []acItem {
	if a.world == nil {
		return nil
	}
	cls := "gCNavZone_PS"
	if a.selQuestType() == g3.QuestArena {
		cls = "gCArena_PS"
	} else if a.selQuestType() == g3.QuestRansack {
		return nil
	}
	seen := map[string]bool{}
	var out []acItem
	for i := range a.world.Inst {
		e := a.world.Inst[i].Ent
		if e.Name == "" || !hasClass(e, cls) || seen[strings.ToLower(e.Name)] {
			continue
		}
		seen[strings.ToLower(e.Name)] = true
		out = append(out, acItem{name: e.Name, label: e.Name, lower: strings.ToLower(e.Name), labelLower: strings.ToLower(e.Name), prio: 1})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].lower < out[j].lower })
	return out
}

// onQuestPointButton: Zielpunkt setzen (Folgen) oder gewaehlten Ort uebernehmen.
func (a *app) onQuestPointButton() {
	q := &a.qst
	t := a.selQuestType()
	if t == g3.QuestFollow {
		a.placeQuestPoint()
		return
	}
	if !usesPlace(t) || a.curQuest() == nil {
		return
	}
	if a.world == nil || a.selected < 0 || int(a.selected) >= len(a.world.Inst) {
		a.flash(T("Bitte zuerst das Objekt in der Welt anklicken."))
		return
	}
	in := &a.world.Inst[a.selected]
	e := in.Ent
	switch t {
	case g3.QuestArena:
		if !hasClass(e, "gCArena_PS") {
			a.flash(fmt.Sprintf(T("„%s“ ist keine Arena. Zonen siehst du mit dem Häkchen „Hilfsobjekte“."), e.Name))
			return
		}
	case g3.QuestReach:
		if !hasClass(e, "gCNavZone_PS") {
			a.flash(fmt.Sprintf(T("„%s“ ist keine Zone. Zonen siehst du mit dem Häkchen „Hilfsobjekte“."), e.Name))
			return
		}
	case g3.QuestRansack:
		if e.Kind == g3.KindNPC || !hasClass(e, "gCInventory_PS") {
			a.flash(fmt.Sprintf(T("„%s“ ist keine Truhe."), e.Name))
			return
		}
		// eindeutigen Namen vergeben, damit die Quest genau diese Truhe findet
		name := e.Name
		focusFrom := e.Name // Anzeigename im Spiel (FO_<Name>) uebernehmen
		count := 0
		for i := range a.world.Inst {
			if strings.EqualFold(a.world.Inst[i].Ent.Name, name) && !a.world.Inst[i].Hidden {
				count++
			}
		}
		if count != 1 || name == "" {
			qq := a.curQuest()
			name = qq.Name() + "_Truhe"
			wf, err := a.editFile(in)
			if err != nil {
				a.flash(fmt.Sprintf(T("Dieses Objekt kann nicht bearbeitet werden: %s"), T(err.Error())))
				return
			}
			k := e.Index
			old := wf.Ents[k].Name
			if err := wf.SetName(k, name); err != nil {
				a.flash(fmt.Sprintf(T("Anlegen nicht möglich: %s"), err.Error()))
				return
			}
			wf.Changed = map[int]bool{k: true}
			a.syncFile(wf)
			a.ed.undo = append(a.ed.undo, func() {
				wf.SetName(k, old)
				wf.Changed = map[int]bool{k: true}
				a.syncFile(wf)
			})
			a.flash(fmt.Sprintf(T("Die Truhe heißt jetzt „%s“ (damit die Quest genau sie findet)."), name))
		}
		a.keepFocusName(name, focusFrom)
		setText(q.place, name)
		q.changed = true
		return
	}
	setText(q.place, e.Name)
	q.changed = true
}

// keepFocusName sorgt dafuer, dass ein umbenanntes Objekt seinen Anzeigenamen
// im Spiel behaelt: Gothic 3 zeigt beim Anvisieren den Text FO_<Name> aus der
// stringtable.ini. Fehlt er fuer den neuen Namen, wird der des alten Namens
// uebernommen (bei Truhen sonst der einer gewoehnlichen Truhe).
func (a *app) keepFocusName(name, from string) {
	st := a.currentStringtable()
	if st == nil {
		return
	}
	if v, ok := a.ed.strs["FO_"+name]; ok && len(v) > 0 {
		return
	}
	if _, ok := g3.StringEntry(st, "FO_"+name); ok {
		return
	}
	for _, k := range []string{from, "G3_Object_Interact_Animated_Chest_01"} {
		if k == "" || strings.EqualFold(k, name) {
			continue
		}
		if v, ok := g3.StringEntry(st, "FO_"+k); ok && len(v) > 0 {
			a.ed.setString("FO_"+name, v)
			return
		}
	}
}

// ensureChestItem sorgt dafuer, dass mindestens n Stueck des Gegenstands fest
// in der Truhe liegen. Rueckgabe: Hinweis fuer den Benutzer ("" = nichts getan).
func (a *app) ensureChestItem(chest, item string, n int) string {
	ii := a.findInstWithClass(chest, "gCInventory_PS")
	if ii < 0 {
		return ""
	}
	in := &a.world.Inst[ii]
	byName, _ := a.itemTemplates()
	t := byName[strings.ToLower(item)]
	if t == nil {
		return ""
	}
	var g [16]byte
	copy(g[:], t.RefGUID[:16])
	wf, err := a.editFile(in)
	if err != nil {
		return ""
	}
	k := in.Ent.Index
	items, err := wf.InventoryItems(k)
	if err != nil {
		return ""
	}
	have := 0
	for _, it := range items {
		if it.Template == g {
			have += it.Amount
		}
	}
	if have >= n {
		return ""
	}
	added := false
	for i := range items {
		if items[i].Template == g {
			items[i].Amount += n - have
			added = true
			break
		}
	}
	if !added {
		items = append(items, g3.StackItem{Template: g, Amount: n - have})
	}
	raw := wf.RawCopy(k)
	if err := wf.SetInventoryItems(k, items); err != nil {
		wf.RestoreRaw(k, raw)
		return fmt.Sprintf(T("Achtung: %s konnte nicht geschrieben werden (%s)."), chest, err.Error())
	}
	a.syncFile(wf)
	a.ed.undo = append(a.ed.undo, func() {
		wf.RestoreRaw(k, raw)
		a.syncFile(wf)
	})
	return fmt.Sprintf(T("In der Truhe lag nicht genug davon – %d × %s wurden fest hineingelegt."), n-have, item)
}

// routineNamed prueft, ob ein NPC einen Tagesablauf dieses Namens hat.
func (a *app) routineNamed(npc, routine string) bool {
	ii := a.findInstWithClass(npc, "gCNavigation_PS")
	if ii < 0 {
		return false
	}
	wf, err := a.editFile(&a.world.Inst[ii])
	if err != nil {
		return false
	}
	list, _, err := wf.Routines(a.world.Inst[ii].Ent.Index)
	if err != nil {
		return false
	}
	for _, r := range list {
		if strings.EqualFold(r.Name, routine) {
			return true
		}
	}
	return false
}

// driveAwayNames: Name des Tagesablaufs nach dem Vertreiben in den vier
// Sprachen (nur Buchstaben, damit er als Name im Spiel gueltig ist).
var driveAwayNames = []string{"Vertrieben", "DrivenAway", "Izgnan", "Wygnany"}

// driveAwayRoutine liefert den vorhandenen Vertrieben-Tagesablauf eines NPCs
// (Name der eingestellten Sprache zuerst, dann die anderen), sonst "".
func (a *app) driveAwayRoutine(npc string) string {
	for _, r := range append([]string{T("Vertrieben")}, driveAwayNames...) {
		if a.routineNamed(npc, r) {
			return r
		}
	}
	return ""
}

// updateZoneOutline liest den Umriss, wenn eine Zone ausgewaehlt ist.
func (a *app) updateZoneOutline() {
	a.zoneLoop = nil
	if a.world == nil || a.selected < 0 || int(a.selected) >= len(a.world.Inst) {
		return
	}
	in := &a.world.Inst[a.selected]
	if !hasClass(in.Ent, "gCNavZone_PS") {
		return
	}
	wf, err := a.editFile(in)
	if err != nil {
		return
	}
	pts, radius, top, bottom, err := wf.ZoneShape(in.Ent.Index)
	if err != nil {
		return
	}
	if len(pts) < 2 && radius > 0 {
		// runde Zone
		pts = nil
		for k := 0; k < 32; k++ {
			ang := float64(k) * 2 * math.Pi / 32
			pts = append(pts, [3]float32{radius * float32(math.Cos(ang)), 0, radius * float32(math.Sin(ang))})
		}
	}
	m := in.Ent.World
	for _, p := range pts {
		a.zoneLoop = append(a.zoneLoop, scene.Vec3{
			p[0]*m[0] + p[1]*m[4] + p[2]*m[8] + m[12],
			p[0]*m[1] + p[1]*m[5] + p[2]*m[9] + m[13],
			p[0]*m[2] + p[1]*m[6] + p[2]*m[10] + m[14],
		})
	}
	if top <= 0 {
		top = 300
	}
	if bottom <= 0 {
		bottom = 100
	}
	a.zoneTop, a.zoneBottom = top, bottom
}

// layoutQstPointRow ordnet die Knoepfe unter "Ort" an (bei "Ort erreichen" zwei).
func (a *app) layoutQstPointRow() {
	q := &a.qst
	if q.pointW == 0 {
		return
	}
	h := a.scale(26)
	if t := a.selQuestType(); (t == g3.QuestReach || t == g3.QuestArena) && a.tab == 3 {
		bw := (q.pointW - a.scale(4)) / 2
		moveWindow(q.zoneBtn, q.pointX, q.pointY, bw, h)
		moveWindow(q.point, q.pointX+bw+a.scale(4), q.pointY, q.pointW-bw-a.scale(4), h)
		pShowWindow.Call(q.zoneBtn, SW_SHOW)
		return
	}
	pShowWindow.Call(q.zoneBtn, SW_HIDE)
	moveWindow(q.point, q.pointX, q.pointY, q.pointW, h)
}

// nearbyZones listet die vorhandenen Zonen (bei Arenakampf nur Arenen) um
// die Bildmitte, die naechste zuerst. Bei "Ort erreichen" steht oben der
// Eintrag fuer eine neue runde Zone in der Bildmitte.
func (a *app) nearbyZones() {
	q := &a.qst
	if a.curQuest() == nil || a.world == nil {
		return
	}
	reach := a.selQuestType() == g3.QuestReach
	cls := "gCNavZone_PS"
	if !reach {
		cls = "gCArena_PS"
	}
	c, ok := a.centerPoint()
	if !ok {
		c = a.cam.Pos
	}
	type cand struct {
		ii   int32
		name string
		d    float64
	}
	best := map[string]cand{}
	for i := range a.world.Inst {
		in := &a.world.Inst[i]
		e := in.Ent
		if in.Hidden || e.Name == "" || !hasClass(e, cls) {
			continue
		}
		dx, dz := float64(in.Center[0]-c[0]), float64(in.Center[2]-c[2])
		d := math.Sqrt(dx*dx+dz*dz) - float64(in.Radius)
		if d < 0 {
			d = 0
		}
		k := strings.ToLower(e.Name)
		if o, ok := best[k]; !ok || d < o.d {
			best[k] = cand{int32(i), e.Name, d}
		}
	}
	var list []cand
	for _, v := range best {
		list = append(list, v)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].d < list[j].d })
	if len(list) > 40 {
		list = list[:40]
	}
	var items []string
	first := 0
	if reach {
		items = append(items, T("+ Neue runde Zone in der Bildmitte anlegen …"))
		first = 1
	}
	if len(list) == 0 && !reach {
		a.flash(T("In der Welt wurden keine Zonen gefunden."))
		return
	}
	for _, v := range list {
		if v.d < 1 {
			items = append(items, fmt.Sprintf(T("%s   (Bildmitte liegt in der Nähe/darin)"), v.name))
		} else {
			items = append(items, fmt.Sprintf(T("%s   (%.0f m entfernt)"), v.name, v.d/100))
		}
	}
	k, ok := a.listBox(T("Zone in der Nähe"), T("Vorhandene Zonen um die Bildmitte, die nächste zuerst. Nach der Auswahl zeigt AdanosEdit ihren Umriss in der Welt."), items, 0)
	if !ok || k < 0 || k >= len(items) {
		return
	}
	if reach && k == 0 {
		a.newQuestZone()
		return
	}
	v := list[k-first]
	setText(q.place, v.name)
	q.changed = true
	a.select_(v.ii, false)
	a.flash(fmt.Sprintf(T("Ort „%s“ eingetragen. Der grüne Umriss zeigt die Zone (F fliegt hin)."), v.name))
}

// newQuestZone legt fuer "Ort erreichen" eine runde Zone in der Bildmitte an
// (oder versetzt sie und aendert den Radius, falls es sie schon gibt). Beim
// Speichern traegt AdanosEdit sie in die Navigationskarte der Mod ein.
func (a *app) newQuestZone() {
	q := &a.qst
	qq := a.curQuest()
	if qq == nil || a.world == nil {
		return
	}
	nav, err := a.navBase()
	if err != nil {
		messageBox(a.hwnd, fmt.Sprintf(T("Eigene Zonen sind nicht möglich: Die Navigationskarte des Spiels konnte nicht gelesen werden (%s)."), err.Error()), T(appTitleDE), MB_ICONWARNING)
		return
	}
	c, ok := a.centerPoint()
	if !ok {
		a.flash(T("Bitte die Kamera so drehen, dass die Bildmitte auf den Boden zeigt."))
		return
	}
	s, ok := a.inputBox(T("Neue Zone"), T("Radius der Zone in Metern (der Held muss in diesen Kreis um die Bildmitte):"), "5")
	if !ok {
		return
	}
	r, err := strconv.ParseFloat(strings.TrimSpace(strings.ReplaceAll(s, ",", ".")), 64)
	if err != nil || r < 0.5 || r > 500 {
		messageBox(a.hwnd, T("Bitte einen Radius zwischen 0,5 und 500 Metern eingeben."), T(appTitleDE), MB_ICONWARNING)
		return
	}
	radius := float32(r * 100)
	if !nav.InGrid(g3.NavZoneInfo{Center: [3]float32{c[0], c[1], c[2]}, Radius: radius}) {
		messageBox(a.hwnd, T("Hier kann keine Zone angelegt werden: Die Stelle liegt außerhalb der Navigationskarte des Spiels."), T(appTitleDE), MB_ICONWARNING)
		return
	}
	var pts [][3]float32
	for k := 0; k < 16; k++ {
		ang := float64(k) * 2 * math.Pi / 16
		pts = append(pts, [3]float32{radius * float32(math.Cos(ang)), 0, radius * float32(math.Sin(ang))})
	}
	name := qq.Name() + "_Zone"
	world := [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, c[0], c[1], c[2], 1}
	// vorhandene Zone anpassen
	if ii := a.findInstWithClass(name, "gCNavZone_PS"); ii >= 0 {
		in := &a.world.Inst[ii]
		wf, err := a.editFile(in)
		if err != nil {
			a.flash(fmt.Sprintf(T("Dieses Objekt kann nicht bearbeitet werden: %s"), T(err.Error())))
			return
		}
		k := in.Ent.Index
		raw := wf.RawCopy(k)
		wf.SetWorld(k, world)
		if err := wf.SetZoneShape(k, pts, radius, 300, 100); err != nil {
			wf.RestoreRaw(k, raw)
			a.flash(fmt.Sprintf(T("Anlegen nicht möglich: %s"), err.Error()))
			return
		}
		wf.Changed = map[int]bool{k: true}
		a.syncFile(wf)
		a.ed.undo = append(a.ed.undo, func() {
			wf.RestoreRaw(k, raw)
			wf.Changed = map[int]bool{k: true}
			a.syncFile(wf)
		})
		setText(q.place, name)
		q.changed = true
		a.select_(ii, false)
		a.flash(fmt.Sprintf(T("Zone „%s“ versetzt (Radius %.1f m). Strg+Z macht es rückgängig."), name, r))
		return
	}
	tpl := npcParts.zone
	if tpl == nil {
		a.flash(T("Die Vorlage für Zonen (Nav_Zone) wurde nicht gefunden."))
		return
	}
	wf, err := a.editFilePath(newObjectFile)
	if err != nil {
		a.flash(fmt.Sprintf(T("Neue Objekte können nicht angelegt werden: %s"), T(err.Error())))
		return
	}
	ci, err := wf.AddFromTemplate(tpl, world)
	if err == nil {
		err = wf.SetName(ci, name)
	}
	if err == nil {
		err = wf.SetZoneShape(ci, pts, radius, 300, 100)
	}
	if err != nil {
		a.flash(fmt.Sprintf(T("Anlegen nicht möglich: %s"), T(err.Error())))
		return
	}
	sector := strings.TrimPrefix(newObjectFile[:strings.LastIndex(newObjectFile, "/")], "G3_World_01/")
	if a.ed.inst[wf.Path] == nil {
		a.ed.inst[wf.Path] = map[int]int32{}
	}
	ii := a.world.Add(wf.Ents[ci], sector)
	if ii >= 0 {
		a.ed.inst[wf.Path][ci] = ii
	}
	wf.Changed = map[int]bool{ci: true}
	a.syncFile(wf)
	a.ed.undo = append(a.ed.undo, func() {
		wf.Delete(ci)
		a.syncFile(wf)
	})
	setText(q.place, name)
	q.changed = true
	if ii >= 0 {
		a.select_(ii, false)
	}
	a.flash(fmt.Sprintf(T("Zone „%s“ angelegt (Radius %.1f m). Sie wird beim Speichern in die Navigationskarte der Mod eingetragen. Strg+Z entfernt sie wieder."), name, r))
}
