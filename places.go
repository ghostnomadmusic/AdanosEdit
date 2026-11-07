//go:build windows

package main

// "Gehe zu": Staedte und Lager anfliegen und das ausgewaehlte Objekt
// (z. B. einen NPC oder den Helden) dorthin versetzen.

import (
	"fmt"
	"math"
	"strings"
	"unsafe"

	"g3viewer/g3"
	"g3viewer/scene"
)

const (
	idGoto = 395 + iota
	idGotoMove
)

type place struct {
	name string // deutscher Name (wird uebersetzt, Eigennamen bleiben)
	pos  scene.Vec3
}

// Mittelpunkte der Bewohner aller Enklaven (aus den Weltdaten berechnet).
var places = []place{
	{"Myrtana: Ardea", scene.Vec3{90961, 5176, -11102}},
	{"Myrtana: Reddock", scene.Vec3{71237, 1281, -12316}},
	{"Myrtana: Kap Dun", scene.Vec3{48292, 4123, -31936}},
	{"Myrtana: Vengard", scene.Vec3{93611, 1949, 34086}},
	{"Myrtana: Montera", scene.Vec3{12220, 7651, 22339}},
	{"Myrtana: Gotha", scene.Vec3{33691, 7728, 49054}},
	{"Myrtana: Faring", scene.Vec3{60236, 8184, 57624}},
	{"Myrtana: Trelis", scene.Vec3{-44844, 563, 10626}},
	{"Myrtana: Nemora", scene.Vec3{-74651, 4985, 1062}},
	{"Myrtana: Geldern", scene.Vec3{-56884, 3478, 43979}},
	{"Myrtana: Okara", scene.Vec3{-4746, 3899, 63690}},
	{"Myrtana: Silden", scene.Vec3{-32705, 506, 73243}},
	{"Nordmar: Wolfsclan", scene.Vec3{46569, 19454, 107811}},
	{"Nordmar: Hammerclan", scene.Vec3{56583, 20384, 128130}},
	{"Nordmar: Feuerclan", scene.Vec3{25231, 19238, 163851}},
	{"Nordmar: Kloster", scene.Vec3{15042, 25307, 150432}},
	{"Nordmar: Große Mine (Orks)", scene.Vec3{50112, 13105, 125674}},
	{"Nordmar: Orklager 1", scene.Vec3{72093, 12150, 80463}},
	{"Nordmar: Orklager 2", scene.Vec3{110814, 10524, 124379}},
	{"Nordmar: Orklager 3", scene.Vec3{49878, 15576, 136092}},
	{"Nordmar: Orklager 4", scene.Vec3{61136, 15187, 156741}},
	{"Nordmar: Orklager 5", scene.Vec3{39957, 14799, 153983}},
	{"Varant: Lago", scene.Vec3{-17958, 616, -53169}},
	{"Varant: Ben Erai", scene.Vec3{-35573, 2329, -67375}},
	{"Varant: Braga", scene.Vec3{-86818, 2050, -51200}},
	{"Varant: Bakaresh", scene.Vec3{40467, 1637, -97559}},
	{"Varant: Ben Sala", scene.Vec3{17421, 4555, -97970}},
	{"Varant: Mora Sul", scene.Vec3{-96317, 3825, -145677}},
	{"Varant: Mora-Sul-Tempel", scene.Vec3{-100644, 1957, -175039}},
	{"Varant: Al Shedim", scene.Vec3{-38547, 1664, -156799}},
	{"Varant: Ishtar", scene.Vec3{-207387, 10108, -155091}},
}

// placeLabel uebersetzt die Gattungswoerter eines Ortsnamens.
func placeLabel(n string) string {
	for _, w := range []string{"Myrtana", "Nordmar", "Varant", "Wolfsclan", "Hammerclan", "Feuerclan", "Kloster", "Große Mine (Orks)", "Orklager", "Mora-Sul-Tempel"} {
		if strings.Contains(n, w) {
			n = strings.Replace(n, w, T(w), 1)
		}
	}
	return n
}

func (a *app) createPlaceControls(mk func(ex uint32, class, text string, style uint32, id int) uintptr) {
	a.hGotoLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	a.hGoto = mk(0, "COMBOBOX", "", CBS_DROPDOWNLIST|WS_VSCROLL|WS_TABSTOP, idGoto)
	a.hGotoMove = mk(0, "BUTTON", "", BS_PUSHBUTTON|WS_TABSTOP, idGotoMove)
	a.applyPlaceTexts()
}

func (a *app) applyPlaceTexts() {
	setText(a.hGotoLbl, T("Gehe zu:"))
	setText(a.hGotoMove, T("Auswahl hierher"))
	items := []string{T("(Ort wählen …)"), T("Spielstart (Held)")}
	for _, p := range places {
		items = append(items, placeLabel(p.name))
	}
	sendMessage(a.hGoto, CB_RESETCONTENT, 0, 0)
	for _, s := range items {
		sendMessage(a.hGoto, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(s))))
	}
	sendMessage(a.hGoto, CB_SETCURSEL, 0, 0)
	sendMessage(a.hGoto, 0x0160, uintptr(a.scale(300)), 0) // CB_SETDROPPEDWIDTH: Liste breiter als das Feld
}

// layoutPlaces setzt die Zeile "Gehe zu" (Hoehe ch).
func (a *app) layoutPlaces(x, y, w int) {
	lw := a.scale(62)
	bw := a.scale(120)
	moveWindow(a.hGotoLbl, x, y+a.scale(5), lw, a.scale(20))
	moveWindow(a.hGoto, x+lw, y, w-lw-bw-a.scale(4), a.scale(420))
	moveWindow(a.hGotoMove, x+w-bw, y, bw, a.scale(26))
}

// placePos liefert die Position des gewaehlten Ortes.
func (a *app) placePos() (scene.Vec3, string, bool) {
	i := int(sendMessage(a.hGoto, CB_GETCURSEL, 0, 0))
	switch {
	case i == 1:
		if a.world != nil {
			for k := range a.world.Inst {
				if in := &a.world.Inst[k]; isHero(in.Ent.Name) && in.Ent.Kind == g3.KindNPC && !in.Hidden {
					p := in.Ent.Pos()
					return scene.Vec3{p[0], p[1], p[2]}, T("Spielstart (Held)"), true
				}
			}
		}
	case i >= 2 && i-2 < len(places):
		return places[i-2].pos, placeLabel(places[i-2].name), true
	}
	return scene.Vec3{}, "", false
}

// gotoPlace fliegt die Kamera zum gewaehlten Ort.
func (a *app) gotoPlace() {
	p, name, ok := a.placePos()
	if !ok {
		return
	}
	// schraeg von oben auf den Ort blicken
	fw := a.cam.Forward()
	fw[1] = 0
	if fw.Len() < 0.01 {
		fw = scene.Vec3{0, 0, -1}
	}
	fw = fw.Norm()
	a.cam.Pos = p.Sub(fw.Scale(5500)).Add(scene.Vec3{0, 5000, 0})
	a.cam.LookAt(p)
	a.flash(fmt.Sprintf(T("Kamera bei %s."), name))
	pSetFocus.Call(a.glHwnd)
}

// moveSelectedToPlace versetzt das ausgewaehlte Objekt an den gewaehlten Ort.
func (a *app) moveSelectedToPlace() {
	p, name, ok := a.placePos()
	if !ok {
		a.flash(T("Bitte zuerst bei „Gehe zu“ einen Ort wählen."))
		return
	}
	in, wf, ok := a.selectedEditable()
	if !ok {
		a.flash(T("Bitte zuerst ein Objekt (z. B. einen NPC oder den Helden) auswählen."))
		return
	}
	k := in.Ent.Index
	old := wf.Ents[k].World
	w := old
	// Schaut die Kamera auf einen Punkt in diesem Ort, dorthin setzen
	// (sonst an den Mittelpunkt des Ortes)
	exact := false
	if c, ok := a.centerPoint(); ok && c.Sub(p).Len() < 20000 {
		p = c
		exact = true
	}
	// Boden suchen (von etwas oberhalb des Punktes nach unten)
	y := p[1]
	if a.loader != nil {
		up := float32(300)
		if exact {
			up = 50 // nicht auf ein Dach darueber springen
		}
		if gy, found := a.world.GroundBelow(p[0], p[1]+up, p[2], a.selected, a.loader.Geometry); found {
			y = gy
		}
	}
	// Ursprung der Figur liegt etwa auf Fusshoehe; Versatz wie bisher beibehalten
	off := old[13] - in.Min[1]
	if in.Ent.Kind != g3.KindNPC || off < 0 || off > 300 {
		off = 0
	}
	w[12], w[13], w[14] = p[0], y+off, p[2]
	dx, dy, dz := w[12]-old[12], w[13]-old[13], w[14]-old[14]
	wf.SetWorld(k, w)
	type moved struct {
		wf  *g3.WorldFile
		k   int
		old [16]float32
	}
	undo := []moved{{wf, k, old}}
	// Routinepunkte eines NPCs mitnehmen?
	if in.Ent.Kind == g3.KindNPC && !isHero(in.Ent.Name) {
		if pts, err := wf.RoutinePoints(k); err == nil {
			var fps []int32
			seen := map[[16]byte]bool{}
			for _, g := range pts {
				if g == ([16]byte{}) || seen[g] {
					continue
				}
				seen[g] = true
				for ii := range a.world.Inst {
					e := a.world.Inst[ii].Ent
					if e.GUID == g && hasClass(e, "gCAIHelper_FreePoint_PS") {
						fps = append(fps, int32(ii))
					}
				}
			}
			if len(fps) > 0 && messageBoxRet(a.hwnd, T("Die Routinepunkte (Arbeit, Freizeit, Schlaf) dieses NPCs liegen noch am alten Ort – dorthin würde er jeden Tag zurücklaufen.\n\nRoutinepunkte mitverschieben? (Bänke, Betten und andere Gegenstände bleiben, wo sie sind.)"), T(appTitleDE), MB_YESNO|MB_ICONQUESTION) == IDYES {
				for _, ii := range fps {
					pin := &a.world.Inst[ii]
					pwf, err := a.editFile(pin)
					if err != nil {
						continue
					}
					pk := pin.Ent.Index
					po := pwf.Ents[pk].World
					pw := po
					pw[12] += dx
					pw[13] += dy
					pw[14] += dz
					pwf.SetWorld(pk, pw)
					a.syncFile(pwf)
					undo = append(undo, moved{pwf, pk, po})
				}
			}
		}
	}
	a.syncFile(wf)
	a.ed.undo = append(a.ed.undo, func() {
		for _, m := range undo {
			m.wf.SetWorld(m.k, m.old)
			a.syncFile(m.wf)
		}
	})
	sel := a.selected
	a.select_(sel, true)
	msg := fmt.Sprintf(T("%s steht jetzt bei %s. Strg+Z macht es rückgängig."), in.Ent.Name, name)
	if isHero(in.Ent.Name) {
		msg += " " + T("Ob das Spiel den Helden beim Start dort lässt, hängt vom Spielbeginn ab – bitte mit einem neuen Spiel prüfen.")
	}
	a.flash(msg)
	_ = math.Pi
}

func hasClass(e *g3.Entity, c string) bool {
	for _, x := range e.Classes {
		if x == c {
			return true
		}
	}
	return false
}
