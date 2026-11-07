//go:build windows

package main

// Autovervollstaendigung fuer Gegenstandsnamen in Eingabefeldern
// (Gegenstaende eines NPCs, Ziel einer Quest). Beim Tippen erscheint unter
// dem Feld eine Liste passender Vorlagen; Pfeiltasten waehlen, Enter oder
// Tab uebernimmt, Esc schliesst.

import (
	"sort"
	"strings"
	"time"
	"unsafe"

	"g3viewer/g3"
)

var (
	pGetWindowRect = user32.NewProc("GetWindowRect")
	pSetWindowPos  = user32.NewProc("SetWindowPos")
)

const (
	idAcList         = 399
	EM_GETSEL        = 0x00B0
	EN_KILLFOCUS     = 0x0200
	SWP_NOACTIVE     = 0x0010
	SWP_SHOW         = 0x0040
	WS_EX_TOOLWINDOW = 0x00000080
	WS_EX_NOACTIVATE = 0x08000000
	VK_UP_           = 0x26
	VK_DOWN_         = 0x28
	VK_TAB_          = 0x09
	WM_SETREDRAW     = 0x000B
)

type acItem struct {
	name, label, lower, labelLower string
	prio                           int // kleiner = weiter oben
}

type autoComplete struct {
	list     uintptr                     // Listenfeld
	edit     uintptr                     // Feld, fuer das die Liste gerade offen ist
	fields   []uintptr                   // Felder mit Autovervollstaendigung
	source   map[uintptr]func() []acItem // Vorschlaege je Feld (nil = Gegenstaende)
	suffix   map[uintptr]string          // wird nach dem Uebernehmen angehaengt
	items    []acItem                    // alle Gegenstaende
	shown    []acItem                    // aktuelle Treffer
	hideAt   time.Time
	applying bool
}

func (a *app) createAutoComplete(mk func(ex uint32, class, text string, style uint32, id int) uintptr) {
	// eigenes Popup-Fenster: liegt immer ueber allen Bedienelementen und
	// nimmt beim Anklicken nicht den Fokus (das Feld bleibt aktiv)
	a.ac.list = createWindow(WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE, "LISTBOX", "",
		WS_POPUP|WS_BORDER|WS_VSCROLL|LBS_NOTIFY|LBS_NOINTEGRALHEIGHT, 0, 0, 10, 10, a.hwnd, 0)
	sendMessage(a.ac.list, WM_SETFONT, a.font, 1)
}

// acItems liefert die Gegenstandsnamen (einmalig aus den Vorlagen).
func (a *app) acItems() []acItem {
	if len(a.ac.items) > 0 || len(a.templates) == 0 {
		return a.ac.items
	}
	a.loadDialogs() // Anzeigenamen (FO_...)
	byName, _ := a.itemTemplates()
	for _, t := range byName {
		it := acItem{name: t.Name, label: t.Name}
		if d := a.displayName(t.Name); d != t.Name {
			it.label = t.Name + "  –  " + d
		}
		it.lower = strings.ToLower(t.Name)
		it.labelLower = strings.ToLower(it.label)
		a.ac.items = append(a.ac.items, it)
	}
	sort.Slice(a.ac.items, func(i, j int) bool { return a.ac.items[i].lower < a.ac.items[j].lower })
	return a.ac.items
}

func (a *app) isACField(h uintptr) bool {
	for _, f := range a.ac.fields {
		if f == h {
			return true
		}
	}
	return false
}

// acToken liefert Anfang und Ende des Gegenstandsnamens an der Schreibmarke.
func acToken(text string, caret int) (start, end int) {
	r := []rune(text)
	if caret > len(r) {
		caret = len(r)
	}
	start = caret
	for start > 0 && r[start-1] != ';' && r[start-1] != ',' && r[start-1] != ' ' {
		start--
	}
	end = caret
	for end < len(r) && r[end] != ';' && r[end] != ',' && r[end] != ' ' {
		end++
	}
	return
}

func editCaret(h uintptr) int {
	r := sendMessage(h, EM_GETSEL, 0, 0)
	return int(uint32(r) >> 16)
}

// onACChange wird bei jeder Aenderung eines Feldes aufgerufen.
func (a *app) onACChange(h uintptr) {
	if a.ac.applying {
		return
	}
	// nur beim Tippen (nicht wenn das Programm das Feld fuellt)
	if f, _, _ := pGetFocus.Call(); f != h {
		return
	}
	// beim Quest-Ziel nur fuer "Gegenstaende bringen"
	if h == a.qst.target && a.selQuestType() != g3.QuestBring && a.selQuestType() != g3.QuestRansack && a.selQuestType() != g3.QuestFree {
		a.hideAC()
		return
	}
	text := getText(h)
	s, e := acToken(text, editCaret(h))
	tok := strings.ToLower(string([]rune(text)[s:e]))
	// nur am Wortanfang eines Eintrags (nicht bei der Anzahl)
	if len(tok) < 2 || (tok[0] >= '0' && tok[0] <= '9') {
		a.hideAC()
		return
	}
	var pre, sub []acItem
	src := a.acItems
	if f := a.ac.source[h]; f != nil {
		src = f
	}
	if h == a.qst.target && a.selQuestType() == g3.QuestFree {
		src = a.enclaveItems
	}
	for _, it := range src() {
		switch {
		case strings.HasPrefix(it.lower, tok):
			pre = append(pre, it)
		case strings.Contains(it.labelLower, tok):
			sub = append(sub, it)
		}
	}
	sort.SliceStable(pre, func(i, j int) bool { return pre[i].prio < pre[j].prio })
	sort.SliceStable(sub, func(i, j int) bool { return sub[i].prio < sub[j].prio })
	// eigene Eintraege (prio 0) vor allen anderen
	var own, rest []acItem
	for _, it := range append(pre, sub...) {
		if it.prio == 0 {
			own = append(own, it)
		} else {
			rest = append(rest, it)
		}
	}
	a.ac.shown = append(own, rest...)
	if len(a.ac.shown) > 300 {
		a.ac.shown = a.ac.shown[:300]
	}
	if len(a.ac.shown) == 0 {
		a.hideAC()
		return
	}
	l := a.ac.list
	sendMessage(l, WM_SETREDRAW, 0, 0)
	sendMessage(l, LB_RESETCONTENT, 0, 0)
	for _, it := range a.ac.shown {
		sendMessage(l, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(it.label))))
	}
	sendMessage(l, LB_SETCURSEL, 0, 0)
	sendMessage(l, WM_SETREDRAW, 1, 0)
	// unter dem Feld anzeigen (ueber der 3D-Ansicht)
	var rc RECT
	pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&rc)))
	pt := POINT{rc.Left, rc.Bottom} // Bildschirmkoordinaten (Popup)
	w := int(rc.Right - rc.Left)
	if w < a.scale(360) {
		w = a.scale(360)
	}
	rows := len(a.ac.shown)
	if rows > 12 {
		rows = 12
	}
	hh := rows*a.scale(18) + a.scale(6)
	pSetWindowPos.Call(l, 0, uintptr(pt.X), uintptr(pt.Y), uintptr(w), uintptr(hh), SWP_NOACTIVE|SWP_SHOW)
	a.ac.edit = h
	pInvalidateRect.Call(l, 0, 1)
}

func (a *app) hideAC() {
	if a.ac.edit != 0 {
		pShowWindow.Call(a.ac.list, SW_HIDE)
		a.ac.edit = 0
	}
}

// acaccept uebernimmt den gewaehlten Eintrag (Index i) in das Feld.
func (a *app) acAccept(i int) {
	h := a.ac.edit
	if h == 0 || i < 0 || i >= len(a.ac.shown) {
		a.hideAC()
		return
	}
	name := a.ac.shown[i].name
	text := []rune(getText(h))
	s, e := acToken(string(text), editCaret(h))
	ins := []rune(name)
	rest := text[e:]
	suf, ok := a.ac.suffix[h]
	if !ok {
		suf = " "
	}
	if suf != "" && (len(rest) == 0 || rest[0] != ' ') {
		ins = append(ins, []rune(suf)...)
	}
	out := append(append(append([]rune{}, text[:s]...), ins...), rest...)
	a.ac.applying = true
	setText(h, string(out))
	a.ac.applying = false
	pos := s + len(ins)
	sendMessage(h, EM_SETSEL, uintptr(pos), uintptr(pos))
	a.hideAC()
	pSetFocus.Call(h)
}

// acKey verarbeitet Tasten im Feld, solange die Liste offen ist.
// true = Taste wurde verbraucht.
func (a *app) acKey(msg *MSG) bool {
	if a.ac.edit == 0 || msg.Hwnd != a.ac.edit || msg.Message != WM_KEYDOWN {
		return false
	}
	l := a.ac.list
	cur := int(sendMessage(l, LB_GETCURSEL, 0, 0))
	switch msg.WParam {
	case VK_DOWN_:
		if cur+1 < len(a.ac.shown) {
			sendMessage(l, LB_SETCURSEL, uintptr(cur+1), 0)
		}
		return true
	case VK_UP_:
		if cur > 0 {
			sendMessage(l, LB_SETCURSEL, uintptr(cur-1), 0)
		}
		return true
	case VK_RETURN_, VK_TAB_:
		a.acAccept(cur)
		return true
	case VK_ESCAPE:
		a.hideAC()
		return true
	}
	return false
}

// acTick schliesst die Liste, wenn weder Feld noch Liste den Fokus haben.
func (a *app) acTick() {
	if a.ac.edit == 0 || a.ac.hideAt.IsZero() || time.Now().Before(a.ac.hideAt) {
		return
	}
	a.ac.hideAt = time.Time{}
	if f, _, _ := pGetFocus.Call(); f != a.ac.edit && f != a.ac.list {
		a.hideAC()
	}
}
