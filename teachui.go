//go:build windows

package main

// Haendler und Lehrer: ob Handel/Lehre von Anfang an offen sind
// (gCDialog_PS) und was ein NPC lehrt (gCNPC_PS TeachAttribs/TeachSkills).
// Gothic 3 zeigt die Auswahl "Handeln" bzw. "Lehren" dann von selbst an;
// spaeter oeffnen laesst sie sich im Dialog mit "> Handel an" / "> Lehren an".

import (
	"fmt"
	"sort"
	"strings"
	"syscall"
	"unsafe"

	"g3viewer/g3"
)

const (
	idTcOK = 1 + iota
	idTcCancel
	idTcTrade
	idTcTeach
	idTcAttr
	idTcSkill
	idTcSet
)

const (
	lbsMultiSel = 0x0008
	lbSetSel    = 0x0185
	lbGetSel    = 0x0187
)

var teachAttribs = []struct{ id, de string }{
	{"STR", "Stärke"}, {"DEX", "Geschick"}, {"INT", "Altes Wissen"}, {"ALC", "Alchemie"},
	{"SMT", "Schmiedekunst"}, {"THF", "Diebeskunst"}, {"HP", "Lebensenergie"}, {"MP", "Mana"}, {"SP", "Ausdauer"},
}

var teachState struct {
	done, ok   bool
	set, trade uintptr
}
var teachClassDone bool

func teachWndProc(hwnd uintptr, msg uint32, w, l uintptr) uintptr {
	switch msg {
	case WM_COMMAND:
		switch loword(w) {
		case idTcOK:
			teachState.ok, teachState.done = true, true
			return 0
		case idTcCancel:
			teachState.done = true
			return 0
		case idTcSet:
			// Haendlerware gewaehlt: Handel gleich mit anhaken
			if hiword(w) == CBN_SELCHANGE && comboSel(teachState.set) > 0 {
				setChecked(teachState.trade, true)
			}
		}
	case WM_CLOSE:
		teachState.done = true
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), w, l)
	return r
}

type perkRef struct {
	label string
	guid  [16]byte
}

// perkList liefert alle Faehigkeiten (Perk_*), sortiert nach Anzeigenamen.
func (a *app) perkList() []perkRef {
	a.loadDialogs() // Texte (FO_It_Perk_...) fuer die Anzeigenamen
	var out []perkRef
	for _, t := range npcParts.perks {
		var g [16]byte
		copy(g[:], t.RefGUID[:16])
		lbl := a.displayName("It_" + t.Name)
		if lbl == "It_"+t.Name {
			lbl = t.Name
		} else {
			lbl += "  (" + t.Name + ")"
		}
		out = append(out, perkRef{lbl, g})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].label < out[j].label })
	return out
}

// editTeach oeffnet das Fenster "Haendler / Lehrer" fuer den gewaehlten NPC.
func (a *app) editTeach() {
	in, wf, ok := a.selectedEditable()
	if !ok || in.Ent.Kind != g3.KindNPC {
		a.flash(T("Bitte zuerst einen NPC in der Welt anklicken."))
		return
	}
	k := in.Ent.Index
	attrs, skills, trade, teach, err := wf.Teach(k)
	if err != nil {
		a.flash(fmt.Sprintf(T("Dieses Objekt kann nicht bearbeitet werden: %s"), T(err.Error())))
		return
	}
	perks := a.perkList()
	curSet := ""
	if ts, err := wf.TreasureSets(k); err == nil {
		for _, s := range ts {
			if g3.TreasureKind(s) == "TS_Trader" {
				curSet = s
			}
		}
	}
	if !teachClassDone {
		if err := registerClass("AdanosEditTeach", syscall.NewCallback(teachWndProc), 0, 16); err != nil {
			return
		}
		teachClassDone = true
	}
	W, H := a.scale(480), a.scale(560)
	var rc RECT
	pGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	pt := POINT{int32((int(rc.Right) - W) / 2), int32((int(rc.Bottom) - H) / 2)}
	if pt.Y < 0 {
		pt.Y = 0
	}
	pClientToScreen.Call(a.hwnd, uintptr(unsafe.Pointer(&pt)))
	title := T("Händler / Lehrer") + " – " + a.displayName(in.Ent.Name)
	h := createWindow(WS_EX_DLGMODAL, "AdanosEditTeach", title, WS_POPUP|WS_CAPTION|WS_SYSMENU|WS_VISIBLE,
		int(pt.X), int(pt.Y), W, H, a.hwnd, 0)
	if h == 0 {
		return
	}
	child := uint32(WS_CHILD | WS_VISIBLE)
	mk := func(ex uint32, class, text string, style uint32, id int, x, y, w, hh int) uintptr {
		c := createWindow(ex, class, text, child|style, a.scale(x), a.scale(y), a.scale(w), a.scale(hh), h, uintptr(id))
		sendMessage(c, WM_SETFONT, a.font, 1)
		return c
	}
	y := 10
	mk(0, "STATIC", T("Händlerware (Schatzset):"), SS_LEFT, 0, 12, y+4, 170, 20)
	sets := append([]string{T("(kein Händler)")}, npcParts.traders...)
	sel := 0
	for i, s := range sets {
		if i > 0 && strings.EqualFold(s, curSet) {
			sel = i
		}
	}
	if curSet != "" && sel == 0 {
		sets = append(sets, curSet)
		sel = len(sets) - 1
	}
	cSet := mk(0, "COMBOBOX", "", CBS_DROPDOWNLIST|WS_VSCROLL|WS_TABSTOP, idTcSet, 185, y, 277, 300)
	comboFill(cSet, sets, sel)
	y += 32
	cTrade := mk(0, "BUTTON", T("Handel von Anfang an offen"), BS_AUTOCHECKBOX|WS_TABSTOP, idTcTrade, 12, y, 450, 22)
	setChecked(cTrade, trade)
	teachState.set, teachState.trade = cSet, cTrade
	y += 24
	cTeach := mk(0, "BUTTON", T("Lehre von Anfang an offen"), BS_AUTOCHECKBOX|WS_TABSTOP, idTcTeach, 12, y, 450, 22)
	setChecked(cTeach, teach)
	y += 30
	mk(0, "STATIC", T("Lehrt Attribute (Klick wählt an/ab):"), SS_LEFT, 0, 12, y, 450, 20)
	y += 22
	lAttr := mk(WS_EX_CLIENTEDGE, "LISTBOX", "", WS_VSCROLL|WS_TABSTOP|lbsMultiSel|LBS_NOINTEGRALHEIGHT, idTcAttr, 12, y, 450, 118)
	has := map[string]bool{}
	for _, s := range attrs {
		has[s] = true
	}
	known := map[string]bool{}
	for i, t := range teachAttribs {
		known[t.id] = true
		sendMessage(lAttr, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(T(t.de)+"  ("+t.id+")"))))
		if has[t.id] {
			sendMessage(lAttr, lbSetSel, 1, uintptr(i))
		}
	}
	y += 126
	mk(0, "STATIC", T("Lehrt Fähigkeiten (Klick wählt an/ab):"), SS_LEFT, 0, 12, y, 450, 20)
	y += 22
	lSkill := mk(WS_EX_CLIENTEDGE, "LISTBOX", "", WS_VSCROLL|WS_TABSTOP|lbsMultiSel|LBS_NOINTEGRALHEIGHT, idTcSkill, 12, y, 450, 200)
	hasSk := map[[16]byte]bool{}
	for _, g := range skills {
		hasSk[g] = true
	}
	knownSk := map[[16]byte]bool{}
	for i, p := range perks {
		knownSk[p.guid] = true
		sendMessage(lSkill, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(p.label))))
		if hasSk[p.guid] {
			sendMessage(lSkill, lbSetSel, 1, uintptr(i))
		}
	}
	y += 208
	mk(0, "STATIC", T("Ohne Häkchen öffnet erst ein Dialog Handel oder Lehre: „> Handel an“ bzw. „> Lehren an“."),
		SS_LEFT, 0, 12, y, 450, 40)
	y += 46
	mk(0, "BUTTON", "OK", BS_DEFPUSH|WS_TABSTOP, idTcOK, 256, y, 100, 30)
	mk(0, "BUTTON", T("Abbrechen"), BS_PUSHBUTTON|WS_TABSTOP, idTcCancel, 362, y, 100, 30)
	var wr, cr RECT
	pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&wr)))
	pGetClientRect.Call(h, uintptr(unsafe.Pointer(&cr)))
	frame := int(wr.Bottom-wr.Top) - int(cr.Bottom)
	pSetWindowPos.Call(h, 0, uintptr(wr.Left), uintptr(wr.Top), uintptr(W), uintptr(a.scale(y+42)+frame), 0x0004)
	pEnableWindow.Call(a.hwnd, 0)
	teachState.done, teachState.ok = false, false
	var msg MSG
	for !teachState.done {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		if msg.Message == WM_KEYDOWN && msg.WParam == VK_ESCAPE {
			teachState.done = true
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	if teachState.ok {
		var na []string
		for i, t := range teachAttribs {
			if sendMessage(lAttr, lbGetSel, uintptr(i), 0) > 0 {
				na = append(na, t.id)
			}
		}
		for _, s := range attrs { // vom Editor nicht angebotene Werte behalten
			if !known[s] {
				na = append(na, s)
			}
		}
		var ns [][16]byte
		for _, g := range skills { // unbekannte Faehigkeiten behalten (Reihenfolge)
			if !knownSk[g] {
				ns = append(ns, g)
			}
		}
		for i, p := range perks {
			if sendMessage(lSkill, lbGetSel, uintptr(i), 0) > 0 {
				ns = append(ns, p.guid)
			}
		}
		nt, nT := isChecked(cTrade), isChecked(cTeach)
		nset := ""
		if i := comboSel(cSet); i > 0 {
			nset = getComboText(cSet, i)
		}
		raw := wf.RawCopy(k)
		var err error
		if !strings.EqualFold(nset, curSet) {
			err = wf.SetTreasureSets(k, map[string]string{"TS_Trader": nset})
		}
		if err == nil {
			err = wf.SetTeach(k, na, ns, nt, nT) // nach den Schatzsets: setzt TradeEnabled endgueltig
		}
		if err != nil {
			wf.RestoreRaw(k, raw)
			a.flash(fmt.Sprintf(T("Anlegen nicht möglich: %s"), err.Error()))
		} else {
			a.syncFile(wf)
			a.ed.undo = append(a.ed.undo, func() {
				wf.RestoreRaw(k, raw)
				a.syncFile(wf)
			})
			a.flash(fmt.Sprintf(T("%s geändert. Strg+Z macht es rückgängig. Gilt in einem neuen Spiel."), in.Ent.Name))
		}
	}
	pEnableWindow.Call(a.hwnd, 1)
	pDestroyWindow.Call(h)
	pSetFocus.Call(a.glHwnd)
	a.select_(a.selected, false)
}

// teachText beschreibt Handel und Lehre eines NPCs (fuer das Infofeld).
func (a *app) teachText(wf *g3.WorldFile, k int) string {
	attrs, skills, trade, teach, err := wf.Teach(k)
	if err != nil || (len(attrs) == 0 && len(skills) == 0 && !trade && !teach) {
		return ""
	}
	s := ""
	if trade {
		s += T("Handel offen") + "\r\n"
	}
	if teach {
		s += T("Lehre offen") + "\r\n"
	}
	if n := len(attrs) + len(skills); n > 0 {
		s += fmt.Sprintf(T("Lehrt %d Attribut(e) und %d Fähigkeit(en)"), len(attrs), len(skills)) + "\r\n"
	}
	return s
}
