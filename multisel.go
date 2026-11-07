//go:build windows

package main

// Mehrfachauswahl: Strg+Klick nimmt Objekte hinzu bzw. heraus, Strg+Ziehen
// waehlt alle Objekte in einem Rahmen. Verschieben (Alt+Maus, Alt+Rad),
// Drehen (R), Loeschen (Entf) und Auf-den-Boden-Setzen (Umschalt+F) wirken
// dann auf alle; Strg+Z macht es in einem Schritt rueckgaengig.
// a.selected bleibt das "Hauptobjekt" (zuletzt gewaehlt, Infofeld, Felder).

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"g3viewer/g3"
	"g3viewer/scene"
)

// Objekte, die groesser sind, werden beim Rahmen nicht mitgewaehlt
// (Gelaende, ganze Gebaeudeteile).
const boxMaxRadius = 2500

type groupMember struct {
	ii int32
	wf *g3.WorldFile
	k  int
}

// inGroup: Mehrfachauswahl aktiv?
func (a *app) inGroup() bool { return len(a.multi) > 1 }

// selection liefert alle gewaehlten Objekte (Hauptobjekt zuerst).
func (a *app) selection() []int32 {
	if !a.inGroup() {
		if a.selected >= 0 {
			return []int32{a.selected}
		}
		return nil
	}
	out := []int32{}
	if a.multi[a.selected] {
		out = append(out, a.selected)
	}
	var rest []int32
	for id := range a.multi {
		if id != a.selected {
			rest = append(rest, id)
		}
	}
	sort.Slice(rest, func(i, j int) bool { return rest[i] < rest[j] })
	return append(out, rest...)
}

func (a *app) clearMulti() { a.multi = nil }

// selectable: darf das Objekt in eine Mehrfachauswahl?
func (a *app) selectable(id int32) bool {
	if a.world == nil || id < 0 || int(id) >= len(a.world.Inst) {
		return false
	}
	in := &a.world.Inst[id]
	return !in.Hidden && in.Ent.Index != 0
}

// toggleMulti: Strg+Klick auf ein Objekt.
func (a *app) toggleMulti(id int32) {
	if !a.selectable(id) {
		return
	}
	if a.multi == nil {
		a.multi = map[int32]bool{}
		if a.selected >= 0 && a.selectable(a.selected) {
			a.multi[a.selected] = true
		}
	}
	if a.multi[id] {
		delete(a.multi, id)
		next := int32(-1)
		for _, s := range a.selection() {
			if s != id {
				next = s
				break
			}
		}
		if len(a.multi) == 0 {
			next = -1
		}
		if len(a.multi) <= 1 {
			a.multi = nil
		}
		a.select_(next, false)
		return
	}
	a.multi[id] = true
	a.select_(id, false)
}

// boxSelect waehlt alle sichtbaren Objekte, deren Mitte im Rahmen liegt
// (zusaetzlich zur bisherigen Auswahl).
func (a *app) boxSelect(vs *viewState, r [4]int32) {
	x0, x1 := min(r[0], r[2]), max(r[0], r[2])
	y0, y1 := min(r[1], r[3]), max(r[1], r[3])
	vp := vs.proj.Mul(vs.view)
	var found []int32
	seen := map[int32]bool{}
	check := func(list []int32) {
		for _, id := range list {
			if seen[id] {
				continue
			}
			seen[id] = true
			in := &a.world.Inst[id]
			if !a.selectable(id) || in.Radius > boxMaxRadius {
				continue
			}
			c := in.Center
			if in.Marker == scene.MarkNPC {
				c = scene.Vec3{in.Ent.World[12], in.Ent.World[13] + 90, in.Ent.World[14]}
			}
			cx := vp[0]*c[0] + vp[4]*c[1] + vp[8]*c[2] + vp[12]
			cy := vp[1]*c[0] + vp[5]*c[1] + vp[9]*c[2] + vp[13]
			cw := vp[3]*c[0] + vp[7]*c[1] + vp[11]*c[2] + vp[15]
			if cw <= 0 {
				continue
			}
			sx := int32((cx/cw + 1) / 2 * float32(vs.width))
			sy := int32((1 - cy/cw) / 2 * float32(vs.height))
			if sx >= x0 && sx <= x1 && sy >= y0 && sy <= y1 {
				found = append(found, id)
			}
		}
	}
	check(a.ren.opaque)
	check(a.ren.blended)
	check(a.ren.markers)
	if len(found) == 0 {
		a.flash(T("Im Rahmen liegt kein Objekt."))
		return
	}
	if a.multi == nil {
		a.multi = map[int32]bool{}
		if a.selected >= 0 && a.selectable(a.selected) {
			a.multi[a.selected] = true
		}
	}
	for _, id := range found {
		a.multi[id] = true
	}
	prim := a.selected
	if !a.multi[prim] {
		prim = found[0]
	}
	if len(a.multi) <= 1 {
		a.multi = nil
	}
	a.select_(prim, false)
	a.flash(fmt.Sprintf(T("%d Objekt(e) ausgewählt."), len(a.selection())))
}

// groupMembers liefert die bearbeitbaren Objekte der Auswahl.
func (a *app) groupMembers() []groupMember {
	var out []groupMember
	bad := 0
	for _, id := range a.selection() {
		in := &a.world.Inst[id]
		if in.Hidden {
			continue
		}
		wf, err := a.editFile(in)
		if err != nil {
			bad++
			continue
		}
		out = append(out, groupMember{id, wf, in.Ent.Index})
	}
	if bad > 0 {
		a.flash(fmt.Sprintf(T("%d Objekt(e) der Auswahl können nicht bearbeitet werden und bleiben unverändert."), bad))
	}
	return out
}

// groupState merkt sich die Weltmatrizen (fuer Rueckgaengig).
func groupState(ms []groupMember) [][16]float32 {
	s := make([][16]float32, len(ms))
	for i, m := range ms {
		s[i] = m.wf.Ents[m.k].World
	}
	return s
}

// syncGroup uebernimmt alle betroffenen Dateien in die Darstellung.
func (a *app) syncGroup(ms []groupMember) {
	done := map[*g3.WorldFile]bool{}
	for _, m := range ms {
		if !done[m.wf] {
			done[m.wf] = true
			a.syncFile(m.wf)
		}
	}
}

// pushGroupUndo legt einen Rueckgaengig-Schritt fuer alle an (falls geaendert).
func (a *app) pushGroupUndo(ms []groupMember, old [][16]float32) {
	changed := false
	for i, m := range ms {
		if m.wf.Ents[m.k].World != old[i] {
			changed = true
		}
	}
	if !changed {
		return
	}
	a.ed.undo = append(a.ed.undo, func() {
		for i, m := range ms {
			m.wf.SetWorld(m.k, old[i])
		}
		a.syncGroup(ms)
	})
}

// translateGroup verschiebt alle gewaehlten Objekte.
func (a *app) translateGroup(ms []groupMember, d scene.Vec3) {
	for _, m := range ms {
		w := m.wf.Ents[m.k].World
		w[12] += d[0]
		w[13] += d[1]
		w[14] += d[2]
		m.wf.SetWorld(m.k, w)
	}
	a.syncGroup(ms)
}

func (a *app) translateGroupUndo(d scene.Vec3) {
	ms := a.groupMembers()
	if len(ms) == 0 {
		return
	}
	old := groupState(ms)
	a.translateGroup(ms, d)
	a.pushGroupUndo(ms, old)
	a.select_(a.selected, false)
}

// rotateGroup dreht alle um ihren gemeinsamen Mittelpunkt (senkrechte Achse).
func (a *app) rotateGroup(deg float64) {
	ms := a.groupMembers()
	if len(ms) == 0 {
		return
	}
	old := groupState(ms)
	var cx, cz float32
	for _, m := range ms {
		w := m.wf.Ents[m.k].World
		cx += w[12]
		cz += w[14]
	}
	cx /= float32(len(ms))
	cz /= float32(len(ms))
	r := deg * math.Pi / 180
	c, s := float32(math.Cos(r)), float32(math.Sin(r))
	rot := [16]float32{c, 0, -s, 0, 0, 1, 0, 0, s, 0, c, 0, 0, 0, 0, 1}
	for _, m := range ms {
		w := m.wf.Ents[m.k].World
		// Position relativ zur Mitte mitdrehen (gleiche Rechnung wie die Achsen)
		w[12] -= cx
		w[14] -= cz
		w = g3.MatMul(w, rot)
		w[12] += cx
		w[14] += cz
		m.wf.SetWorld(m.k, w)
	}
	a.syncGroup(ms)
	a.pushGroupUndo(ms, old)
	a.select_(a.selected, false)
}

// deleteGroup loescht alle gewaehlten Objekte in einem Schritt.
func (a *app) deleteGroup() {
	ms := a.groupMembers()
	if len(ms) == 0 {
		return
	}
	npcs := 0
	for _, m := range ms {
		if a.world.Inst[m.ii].Ent.Kind == g3.KindNPC {
			npcs++
		}
	}
	msg := fmt.Sprintf(T("%d Objekte löschen?"), len(ms))
	if npcs > 0 {
		msg += "\n\n" + fmt.Sprintf(T("Darunter sind %d NPC(s). Skripte oder Quests können auf sie verweisen und im Spiel Fehler auslösen."), npcs)
	}
	if messageBoxRet(a.hwnd, msg, T(appTitleDE), MB_YESNO|MB_ICONQUESTION) != IDYES {
		return
	}
	var done []groupMember
	for _, m := range ms {
		if m.k == 0 {
			continue
		}
		if err := m.wf.Delete(m.k); err == nil {
			done = append(done, m)
		}
	}
	a.syncGroup(done)
	a.ed.undo = append(a.ed.undo, func() {
		for _, m := range done {
			m.wf.Undelete(m.k)
		}
		a.syncGroup(done)
	})
	a.clearMulti()
	a.select_(-1, false)
	a.flash(fmt.Sprintf(T("%d Objekte gelöscht (Strg+Z macht es rückgängig)."), len(done)))
}

// dropGroup setzt jedes gewaehlte Objekt auf den Boden darunter.
func (a *app) dropGroup() {
	ms := a.groupMembers()
	if len(ms) == 0 || a.loader == nil {
		return
	}
	old := groupState(ms)
	setText(a.hStatus, T("Boden wird gesucht …"))
	ok, miss := 0, 0
	for _, m := range ms {
		if dy, found := a.groundOffset(m.ii); found {
			w := m.wf.Ents[m.k].World
			w[13] += dy
			m.wf.SetWorld(m.k, w)
			ok++
		} else {
			miss++
		}
	}
	a.lastStatusText = ""
	a.syncGroup(ms)
	a.pushGroupUndo(ms, old)
	a.select_(a.selected, false)
	if miss > 0 {
		a.flash(fmt.Sprintf(T("%d Objekt(e) auf den Boden gesetzt, bei %d wurde kein Boden gefunden."), ok, miss))
	} else {
		a.flash(fmt.Sprintf(T("%d Objekt(e) auf den Boden gesetzt."), ok))
	}
}

// groupInfo beschreibt die Mehrfachauswahl fuer das Infofeld.
func (a *app) groupInfo() string {
	sel := a.selection()
	var b strings.Builder
	fmt.Fprintf(&b, T("%d Objekte ausgewählt"), len(sel))
	b.WriteString("\r\n\r\n")
	b.WriteString(T("Alt + linke Maustaste verschiebt alle, Alt + Mausrad hebt/senkt sie, R dreht sie um ihre gemeinsame Mitte, Umschalt+F setzt jedes auf den Boden, Entf löscht alle. Strg+Klick nimmt Objekte hinzu oder heraus, Esc hebt die Auswahl auf."))
	b.WriteString("\r\n\r\n")
	for i, id := range sel {
		if i >= 40 {
			fmt.Fprintf(&b, T("… und %d weitere"), len(sel)-i)
			b.WriteString("\r\n")
			break
		}
		e := a.world.Inst[id].Ent
		n := e.Name
		if n == "" {
			n = e.Mesh
		}
		if i == 0 {
			n += "  " + T("(Hauptobjekt)")
		}
		b.WriteString("• " + n + "\r\n")
	}
	return b.String()
}

// drawBoxRect zeichnet den Auswahlrahmen (Bildschirmkoordinaten).
func drawBoxRect(r [4]int32, w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	glMatrixMode(GL_PROJECTION)
	glPushMatrix()
	glLoadIdentity()
	glMatrixMode(GL_MODELVIEW)
	glPushMatrix()
	glLoadIdentity()
	glDisable(GL_LIGHTING)
	glDisable(GL_TEXTURE_2D)
	glDisable(GL_FOG)
	glDisable(GL_DEPTH_TEST)
	glDisableClientState(GL_VERTEX_ARRAY)
	glDisableClientState(GL_NORMAL_ARRAY)
	glDisableClientState(GL_TEXTURE_COORD_ARRAY)
	glLineWidth(1.5)
	c := [4]float32{1, 0.85, 0.1, 1}
	glColor4fv(&c)
	nx := func(x int32) float32 { return float32(x)/float32(w)*2 - 1 }
	ny := func(y int32) float32 { return 1 - float32(y)/float32(h)*2 }
	pts := [][3]float32{{nx(r[0]), ny(r[1]), 0}, {nx(r[2]), ny(r[1]), 0}, {nx(r[2]), ny(r[3]), 0}, {nx(r[0]), ny(r[3]), 0}}
	glBegin(GL_LINE_LOOP)
	for i := range pts {
		glVertex3fv(&pts[i])
	}
	glEnd()
	glEnable(GL_DEPTH_TEST)
	glLineWidth(1)
	glMatrixMode(GL_PROJECTION)
	glPopMatrix()
	glMatrixMode(GL_MODELVIEW)
	glPopMatrix()
}

// groupOnlyOne: Kopieren geht vorerst nur mit einem Objekt.
func (a *app) groupOnlyOne() bool {
	if a.inGroup() {
		a.flash(T("Kopieren geht bisher nur mit einem einzelnen Objekt. Klicke ein Objekt ohne Strg an."))
		return true
	}
	return false
}

// finishMove schliesst das Verschieben mit Alt + Maus ab (Rueckgaengig).
func (a *app) finishMove() {
	if a.moveGroup != nil {
		a.pushGroupUndo(a.moveGroup, a.moveGroupOld)
		a.moveGroup, a.moveGroupOld = nil, nil
		return
	}
	a.endMove(a.moveOld)
}
