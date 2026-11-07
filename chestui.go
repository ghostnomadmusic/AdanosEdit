//go:build windows

package main

// Truhen und Schloesser: Schatzsets (zufaelliger Inhalt), feste Gegenstaende,
// Schloss (offen, leicht ... schwer, nur mit Schluessel) und Schluessel.

import (
	"fmt"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"g3viewer/g3"
	"g3viewer/scene"
)

const (
	idChOK = 1 + iota
	idChCancel
	idChSet1
	idChSet2
	idChItems
	idChLock
	idChKey
)

var chestState struct {
	done, ok bool
}

var chestClassDone bool

func chestWndProc(hwnd uintptr, msg uint32, w, l uintptr) uintptr {
	switch msg {
	case WM_COMMAND:
		switch loword(w) {
		case idChOK:
			chestState.ok, chestState.done = true, true
			return 0
		case idChCancel:
			chestState.done = true
			return 0
		}
	case WM_CLOSE:
		chestState.done = true
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), w, l)
	return r
}

var lockNames = []string{"offen", "leicht zu knacken", "mittel zu knacken", "schwer zu knacken", "nur mit Schlüssel"}

// editChest oeffnet das Fenster fuer Inhalt und Schloss des gewaehlten Objekts.
func (a *app) editChest() {
	in, wf, ok := a.selectedEditable()
	if !ok {
		a.flash(T("Bitte zuerst eine Truhe (oder Tür) in der Welt anklicken."))
		return
	}
	k := in.Ent.Index
	hasInv := in.Ent.Kind != g3.KindNPC && wf.HasClass(k, "gCInventory_PS")
	hasLock := wf.HasClass(k, "gCLock_PS")
	if !hasInv && !hasLock {
		a.flash(fmt.Sprintf(T("„%s“ hat weder Inhalt noch Schloss."), in.Ent.Name))
		return
	}
	byName, byGUID := a.itemTemplates()
	var sets []string
	var items []g3.StackItem
	if hasInv {
		sets, _ = wf.ContainerSets(k)
		items, _ = wf.InventoryItems(k)
	}
	var lock g3.Lock
	if hasLock {
		lock, _ = wf.LockOf(k)
	}
	// Fenster
	if !chestClassDone {
		if err := registerClass("AdanosEditChest", syscall.NewCallback(chestWndProc), 0, 16); err != nil {
			return
		}
		chestClassDone = true
	}
	W, H := a.scale(480), a.scale(330)
	var rc RECT
	pGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	pt := POINT{int32((int(rc.Right) - W) / 2), int32((int(rc.Bottom) - H) / 2)}
	pClientToScreen.Call(a.hwnd, uintptr(unsafe.Pointer(&pt)))
	title := T("Truhe / Schloss") + " – " + in.Ent.Name
	h := createWindow(WS_EX_DLGMODAL, "AdanosEditChest", title, WS_POPUP|WS_CAPTION|WS_SYSMENU|WS_VISIBLE,
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
	combo := func(id, x, y, w int, list []string, sel int) uintptr {
		c := mk(0, "COMBOBOX", "", CBS_DROPDOWNLIST|WS_VSCROLL|WS_TABSTOP, id, x, y, w, 260)
		comboFill(c, list, sel)
		return c
	}
	y := 10
	var cSet1, cSet2, eItems uintptr
	if hasInv {
		mk(0, "STATIC", T("Zufälliger Inhalt (Schatzsets):"), SS_LEFT, 0, 12, y, 450, 20)
		y += 22
		list := append([]string{T("(keiner)")}, npcParts.chestSets...)
		pick := func(i int) int {
			if i >= len(sets) {
				return 0
			}
			for j, s := range list {
				if strings.EqualFold(s, sets[i]) {
					return j
				}
			}
			list = append(list, sets[i])
			return len(list) - 1
		}
		s1, s2 := pick(0), pick(1)
		cSet1 = combo(idChSet1, 12, y, 220, list, s1)
		cSet2 = combo(idChSet2, 240, y, 222, list, s2)
		y += 34
		mk(0, "STATIC", T("Feste Gegenstände (z. B. It_Gold 500; It_Apple 3):"), SS_LEFT, 0, 12, y, 450, 20)
		y += 22
		var parts []string
		for _, it := range items {
			n := "?"
			if t := byGUID[it.Template]; t != nil {
				n = t.Name
			}
			parts = append(parts, fmt.Sprintf("%s %d", n, it.Amount))
		}
		eItems = mk(WS_EX_CLIENTEDGE, "EDIT", strings.Join(parts, "; "), ES_AUTOHSCROLL|WS_TABSTOP, idChItems, 12, y, 450, 24)
		y += 36
	}
	var cLock, eKey uintptr
	if hasLock {
		mk(0, "STATIC", T("Schloss:"), SS_LEFT, 0, 12, y+4, 100, 20)
		var ln []string
		for _, s := range lockNames {
			ln = append(ln, T(s))
		}
		sel := 0
		if lock.Locked {
			switch {
			case lock.Difficulty >= 1 && lock.Difficulty <= 3:
				sel = lock.Difficulty
			default:
				sel = 4
			}
		}
		cLock = combo(idChLock, 120, y, 342, ln, sel)
		y += 34
		mk(0, "STATIC", T("Schlüssel (Vorlagenname, leer = keiner):"), SS_LEFT, 0, 12, y, 450, 20)
		y += 22
		key := ""
		if lock.Key != ([16]byte{}) {
			key = "?"
			if t := byGUID[lock.Key]; t != nil {
				key = t.Name
			}
		}
		eKey = mk(WS_EX_CLIENTEDGE, "EDIT", key, ES_AUTOHSCROLL|WS_TABSTOP, idChKey, 12, y, 450, 24)
		y += 36
	}
	mk(0, "STATIC", T("Neue Inhalte gelten in einem neuen Spiel."), SS_LEFT, 0, 12, y, 450, 20)
	y += 28
	mk(0, "BUTTON", "OK", BS_DEFPUSH|WS_TABSTOP, idChOK, 256, y, 100, 30)
	mk(0, "BUTTON", T("Abbrechen"), BS_PUSHBUTTON|WS_TABSTOP, idChCancel, 362, y, 100, 30)
	// Fensterhoehe anpassen
	var wr RECT
	pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&wr)))
	var cr RECT
	pGetClientRect.Call(h, uintptr(unsafe.Pointer(&cr)))
	frame := int(wr.Bottom-wr.Top) - int(cr.Bottom)
	pSetWindowPos.Call(h, 0, uintptr(wr.Left), uintptr(wr.Top), uintptr(W), uintptr(a.scale(y+42)+frame), 0x0004)
	pEnableWindow.Call(a.hwnd, 0)
	for {
		chestState.done, chestState.ok = false, false
		var msg MSG
		for !chestState.done {
			r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(r) <= 0 {
				break
			}
			if msg.Message == WM_KEYDOWN && msg.WParam == VK_ESCAPE {
				chestState.done = true
				break
			}
			if msg.Message == WM_KEYDOWN && msg.WParam == VK_RETURN_ {
				chestState.ok, chestState.done = true, true
				break
			}
			pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}
		if !chestState.ok {
			break
		}
		// Eingaben pruefen
		bad := func(m string) {
			pEnableWindow.Call(a.hwnd, 1)
			messageBox(h, m, T(appTitleDE), MB_ICONWARNING)
			pEnableWindow.Call(a.hwnd, 0)
		}
		var nsets []string
		var nitems []g3.StackItem
		okIn := true
		if hasInv {
			for _, c := range []uintptr{cSet1, cSet2} {
				if i := comboSel(c); i > 0 {
					txt := getComboText(c, i)
					nsets = append(nsets, txt)
				}
			}
			// weitere (vom Editor nicht angezeigte) Sets behalten
			if len(sets) > 2 {
				nsets = append(nsets, sets[2:]...)
			}
			for _, part := range strings.FieldsFunc(getText(eItems), func(r rune) bool { return r == ';' || r == ',' }) {
				f := strings.Fields(part)
				if len(f) == 0 {
					continue
				}
				cnt := 1
				if len(f) > 1 {
					v, err := strconv.Atoi(f[1])
					if err != nil || v < 1 {
						bad(fmt.Sprintf(T("„%s“ ist keine gültige Zahl."), f[1]))
						okIn = false
						break
					}
					cnt = v
				}
				t := byName[strings.ToLower(f[0])]
				if t == nil {
					bad(fmt.Sprintf(T("Den Gegenstand „%s“ kenne ich nicht (Vorlagenname wie It_Gold oder It_Apple?)."), f[0]))
					okIn = false
					break
				}
				var g [16]byte
				copy(g[:], t.RefGUID[:16])
				nitems = append(nitems, g3.StackItem{Template: g, Amount: cnt})
			}
		}
		var nlock g3.Lock
		if okIn && hasLock {
			switch i := comboSel(cLock); {
			case i <= 0:
				nlock = g3.Lock{Locked: false, Difficulty: -1}
			case i >= 1 && i <= 3:
				nlock = g3.Lock{Locked: true, Difficulty: i}
			default:
				nlock = g3.Lock{Locked: true, Difficulty: 0}
			}
			if kn := strings.TrimSpace(getText(eKey)); kn != "" {
				t := byName[strings.ToLower(kn)]
				if t == nil {
					bad(fmt.Sprintf(T("Den Gegenstand „%s“ kenne ich nicht (Vorlagenname wie It_Gold oder It_Apple?)."), kn))
					okIn = false
				} else {
					copy(nlock.Key[:], t.RefGUID[:16])
				}
			}
			if okIn && nlock.Locked && nlock.Difficulty == 0 && nlock.Key == ([16]byte{}) {
				bad(T("„Nur mit Schlüssel“ braucht einen Schlüssel (z. B. It_Key_Fabio)."))
				okIn = false
			}
		}
		if !okIn {
			continue
		}
		// Anwenden
		raw := wf.RawCopy(k)
		var err error
		if hasInv {
			if err = wf.SetContainerSets(k, nsets); err == nil {
				err = wf.SetInventoryItems(k, nitems)
			}
		}
		if err == nil && hasLock {
			err = wf.SetLock(k, nlock)
		}
		if err != nil {
			wf.RestoreRaw(k, raw)
			bad(fmt.Sprintf(T("Anlegen nicht möglich: %s"), err.Error()))
			continue
		}
		a.syncFile(wf)
		a.ed.undo = append(a.ed.undo, func() {
			wf.RestoreRaw(k, raw)
			a.syncFile(wf)
		})
		a.flash(fmt.Sprintf(T("%s geändert. Strg+Z macht es rückgängig. Gilt in einem neuen Spiel."), in.Ent.Name))
		break
	}
	pEnableWindow.Call(a.hwnd, 1)
	pDestroyWindow.Call(h)
	pSetFocus.Call(a.glHwnd)
	a.select_(a.selected, false)
}

// getComboText liefert den Text eines Eintrags.
func getComboText(h uintptr, i int) string {
	n := int(sendMessage(h, 0x0149, uintptr(i), 0)) // CB_GETLBTEXTLEN
	if n <= 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	sendMessage(h, 0x0148, uintptr(i), uintptr(unsafe.Pointer(&buf[0]))) // CB_GETLBTEXT
	return syscall.UTF16ToString(buf)
}

// chestText beschreibt Inhalt und Schloss des Objekts (fuer das Infofeld).
func (a *app) chestText(in *scene.Instance) string {
	if in.Ent.Kind == g3.KindNPC {
		return ""
	}
	has := false
	for _, c := range in.Ent.Classes {
		if c == "gCLock_PS" || c == "gCInventory_PS" {
			has = true
		}
	}
	if !has {
		return ""
	}
	wf, err := a.editFile(in)
	if err != nil {
		return ""
	}
	k := in.Ent.Index
	_, byGUID := a.itemTemplates()
	var b strings.Builder
	if sets, err := wf.ContainerSets(k); err == nil {
		s := T("(keiner)")
		if len(sets) > 0 {
			s = strings.Join(sets, ", ")
		}
		fmt.Fprintf(&b, T("Zufälliger Inhalt: %s")+"\r\n", s)
		if items, err := wf.InventoryItems(k); err == nil && len(items) > 0 {
			var p []string
			for _, it := range items {
				n := "?"
				if t := byGUID[it.Template]; t != nil {
					n = t.Name
				}
				p = append(p, fmt.Sprintf("%s %d", n, it.Amount))
			}
			fmt.Fprintf(&b, T("Feste Gegenstände: %s")+"\r\n", strings.Join(p, "; "))
		}
	}
	if l, err := wf.LockOf(k); err == nil {
		st := T("offen")
		if l.Locked {
			if l.Difficulty >= 1 && l.Difficulty <= 3 {
				st = T(lockNames[l.Difficulty])
			} else {
				st = T("nur mit Schlüssel")
			}
		}
		if l.Key != ([16]byte{}) {
			n := "?"
			if t := byGUID[l.Key]; t != nil {
				n = t.Name
			}
			st += ", " + fmt.Sprintf(T("Schlüssel: %s"), n)
		}
		fmt.Fprintf(&b, T("Schloss: %s")+"\r\n", st)
	}
	if b.Len() > 0 {
		b.WriteString(T("(ändern mit „Truhe / Schloss …“)") + "\r\n")
	}
	return b.String()
}
