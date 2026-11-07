//go:build windows

package main

// "Mod pruefen": sucht typische Fehler, die man im Spiel erst spaet bemerkt
// (Dialoge ohne NPC, Quests ohne Auftraggeber, Ereignisse, die nie gesetzt
// werden, unbekannte Gegenstaende, Zonen ausserhalb der Navigationskarte).
// Laeuft auf Knopfdruck und vor jedem Speichern (nur wenn etwas gefunden wird).

import (
	"fmt"
	"sort"
	"strings"
	"syscall"
	"unsafe"

	"g3viewer/g3"
)

const idModCheck = 420

const (
	idMcList = 1 + iota
	idMcGo
	idMcSave
	idMcClose
)

type checkIssue struct {
	err  bool   // true = Fehler (wirkt im Spiel nicht), false = Hinweis
	text string // Beschreibung
	jump func() // Hingehen (nil = nicht moeglich)
}

// modCheck prueft die Mod und liefert die gefundenen Probleme.
func (a *app) modCheck() []checkIssue {
	if a.world == nil {
		return nil
	}
	a.loadDialogs()
	a.loadQuests()
	d := &a.dlg
	var out []checkIssue
	add := func(err bool, jump func(), f string, v ...any) {
		out = append(out, checkIssue{err, fmt.Sprintf(f, v...), jump})
	}
	// Objekte der Welt nach Namen
	names := map[string]int32{}
	for i := range a.world.Inst {
		in := &a.world.Inst[i]
		if in.Hidden || in.Ent.Name == "" {
			continue
		}
		if _, ok := names[strings.ToLower(in.Ent.Name)]; !ok {
			names[strings.ToLower(in.Ent.Name)] = int32(i)
		}
	}
	exists := func(n string) bool {
		if strings.EqualFold(n, "PC_Hero") {
			return true
		}
		_, ok := names[strings.ToLower(n)]
		return ok
	}
	items := a.itemNames()
	known := a.knownEvents()
	quests := map[string]bool{}
	for _, qq := range a.qst.quests {
		quests[strings.ToLower(qq.Name())] = true
	}
	// eigene und geaenderte Dialoge
	var paths []string
	for lp, in := range d.infos {
		if isOwnInfo(in) || a.isModified(d.paths[lp]) {
			paths = append(paths, lp)
		}
	}
	sort.Strings(paths)
	for _, lp := range paths {
		in := d.infos[lp]
		p := d.paths[lp]
		if p == "" {
			p = lp
		}
		jump := a.jumpDialog(in.Owner, p)
		title := strings.TrimSpace(strings.TrimLeft(a.dlgTitle(dlgItem{info: in, path: p}), "•»▸⊘ "))
		if !exists(in.Owner) {
			add(true, jump, T("Dialog „%s“: Der NPC „%s“ existiert in der Welt nicht (mehr) – der Dialog erscheint nie."), title, in.Owner)
		}
		if isDisabled(in) {
			continue
		}
		origEv := map[string]bool{}
		if !isOwnInfo(in) {
			// bei Original-Dialogen nur selbst hinzugefuegte Ereignisse pruefen
			// (manche setzt das Spiel per Skript)
			if o := a.originalInfo(p); o != nil {
				for _, ev := range splitEvents(o.CondPlayerKnows) {
					origEv[ev] = true
				}
			}
		}
		for _, ev := range splitEvents(in.CondPlayerKnows) {
			if !known[ev] && !origEv[ev] {
				add(true, jump, T("Dialog „%s“: „Erst nach“ wartet auf „%s“, das kein Dialog setzt – der Dialog erscheint nie."), title, ev)
			}
		}
		if in.Parent != "" {
			found := false
			for _, o := range d.infos {
				if strings.EqualFold(o.Name, in.Parent) && strings.EqualFold(o.Owner, in.Owner) {
					found = true
				}
			}
			if !found {
				add(true, jump, T("Dialog „%s“: Der übergeordnete Dialog „%s“ fehlt – der Dialog erscheint nie."), title, in.Parent)
			}
		}
		if in.Quest != "" && !quests[strings.ToLower(in.Quest)] {
			add(true, jump, T("Dialog „%s“: Die Quest „%s“ gibt es nicht."), title, in.Quest)
		}
		for _, c := range in.Script {
			switch c.Cmd {
			case "Give":
				if items != nil && c.ID1 != "" && !items[strings.ToLower(c.ID1)] {
					add(true, jump, T("Dialog „%s“: Den Gegenstand „%s“ gibt es nicht."), title, c.ID1)
				}
			case "RunQuest", "CloseQuest":
				if c.ID1 != "" && !quests[strings.ToLower(c.ID1)] {
					add(true, jump, T("Dialog „%s“: Die Quest „%s“ gibt es nicht."), title, c.ID1)
				}
			}
		}
		if items != nil {
			for _, it := range splitEvents(in.CondItems) {
				if !items[strings.ToLower(it)] {
					add(true, jump, T("Dialog „%s“: Den Gegenstand „%s“ (bei „Held hat“) gibt es nicht."), title, it)
				}
			}
		}
	}
	// eigene Quests
	var own []string
	for _, qq := range a.qst.quests {
		if n := qq.Name(); isOwnQuest(n) {
			own = append(own, n)
		}
	}
	sort.Strings(own)
	for _, n := range own {
		qq := a.questByName(n)
		jump := a.jumpQuest(n)
		title := a.questTitle(n)
		started := false
		for _, in := range d.infos {
			if isDisabled(in) {
				continue
			}
			if strings.EqualFold(in.Quest, n) && in.ConditionType == g3.CondActivator {
				started = true
			}
			for _, c := range in.Script {
				if c.Cmd == "RunQuest" && strings.EqualFold(c.ID1, n) {
					started = true
				}
			}
		}
		if !started {
			add(true, jump, T("Quest „%s“: Kein Dialog vergibt sie – sie startet im Spiel nie. (Im Reiter Quests „Passende Dialoge erzeugen“.)"), title)
		}
		for _, f := range splitEvents(qq.Get("FinishedQuests")) {
			if !quests[strings.ToLower(f)] {
				add(true, jump, T("Quest „%s“: Die vorausgesetzte Quest „%s“ gibt es nicht."), title, f)
			}
		}
		typ := qq.GetInt("Type")
		dest := qq.Get("DestinationEntity")
		for _, e := range splitEvents(qq.Get("DeliveryEntities")) {
			switch typ {
			case g3.QuestBring, g3.QuestRansack:
				if items != nil && !items[strings.ToLower(e)] {
					add(true, jump, T("Quest „%s“: Den Gegenstand „%s“ gibt es nicht."), title, e)
				}
			case g3.QuestKill, g3.QuestDefeat, g3.QuestDriveAway, g3.QuestSpell, g3.QuestBringNPC, g3.QuestFollow:
				if !exists(e) {
					add(true, jump, T("Quest „%s“: „%s“ gibt es in der Welt nicht (mehr)."), title, e)
				}
			}
		}
		if dest != "" && typ != g3.QuestFree && !exists(dest) {
			add(true, jump, T("Quest „%s“: Das Ziel „%s“ gibt es in der Welt nicht (mehr)."), title, dest)
		}
	}
	// eigene Zonen ausserhalb der Navigationskarte
	if base, err := a.navBase(); err == nil {
		for i := range a.world.Inst {
			in := &a.world.Inst[i]
			if in.Hidden || !hasClass(in.Ent, "gCNavZone_PS") || base.HasZone(in.Ent.GUID) {
				continue
			}
			if z, ok := a.zoneNavInfo(i); ok && !base.InGrid(z) {
				add(true, a.jumpObject(int32(i)), T("Zone „%s“ liegt außerhalb der Navigationskarte und wird im Spiel nicht erkannt."), in.Ent.Name)
			}
		}
	}
	// andere Mods mit eigener Navigationskarte
	if others := a.otherNavMapMods(); len(others) > 0 {
		add(false, nil, T("Andere aktive Mods bringen eine eigene Navigationskarte mit (%s). Nur eine kann gelten – eigene Zonen funktionieren dann evtl. nicht."), strings.Join(others, ", "))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].err && !out[j].err })
	return out
}

// --- Hingehen -------------------------------------------------------------

func (a *app) jumpDialog(owner, path string) func() {
	return func() {
		a.dlg.keepOwner = true
		a.setTab(2)
		d := &a.dlg
		d.owner = owner
		d.cur = -1
		a.fillDlgNPCs()
		a.fillDlgList()
		for i, it := range d.items {
			if strings.EqualFold(it.path, path) {
				d.cur = i
				sendMessage(d.list, LB_SETCURSEL, uintptr(i), 0)
			}
		}
		a.showDialog()
	}
}

func (a *app) jumpQuest(name string) func() {
	return func() {
		a.setTab(3)
		q := &a.qst
		for i, n := range q.names {
			if n == name {
				q.cur = i
			}
		}
		a.fillQstList()
		a.showQuest()
	}
}

func (a *app) jumpObject(id int32) func() {
	return func() {
		a.setTab(0)
		a.clearMulti()
		a.select_(id, true)
	}
}

// --- Fenster ----------------------------------------------------------------

var mcState struct {
	done   bool
	result int // idMcGo, idMcSave, idMcClose
	list   uintptr
}
var mcClassDone bool

func mcWndProc(hwnd uintptr, msg uint32, w, l uintptr) uintptr {
	switch msg {
	case WM_COMMAND:
		switch loword(w) {
		case idMcGo, idMcSave, idMcClose:
			mcState.result, mcState.done = int(loword(w)), true
			return 0
		case idMcList:
			if hiword(w) == LBN_DBLCLK {
				mcState.result, mcState.done = idMcGo, true
				return 0
			}
		}
	case WM_CLOSE:
		mcState.result, mcState.done = idMcClose, true
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), w, l)
	return r
}

// showModCheck prueft die Mod und zeigt das Ergebnis. beforeSave: mit Knopf
// "Trotzdem speichern"; Rueckgabe true = speichern.
func (a *app) showModCheck(beforeSave bool) bool {
	if !a.commitDialog() || !a.commitQuest() {
		return false
	}
	issues := a.modCheck()
	if len(issues) == 0 {
		if !beforeSave {
			messageBox(a.hwnd, T("Keine Probleme gefunden."), T(appTitleDE), MB_ICONINFORMATION)
		}
		return true
	}
	if !mcClassDone {
		if err := registerClass("AdanosEditCheck", syscall.NewCallback(mcWndProc), 0, 16); err != nil {
			return true
		}
		mcClassDone = true
	}
	W, H := a.scale(620), a.scale(440)
	var rc RECT
	pGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	pt := POINT{int32((int(rc.Right) - W) / 2), int32((int(rc.Bottom) - H) / 2)}
	if pt.Y < 0 {
		pt.Y = 0
	}
	pClientToScreen.Call(a.hwnd, uintptr(unsafe.Pointer(&pt)))
	nErr := 0
	for _, is := range issues {
		if is.err {
			nErr++
		}
	}
	h := createWindow(WS_EX_DLGMODAL, "AdanosEditCheck", T("Mod prüfen"), WS_POPUP|WS_CAPTION|WS_SYSMENU|WS_VISIBLE,
		int(pt.X), int(pt.Y), W, H, a.hwnd, 0)
	if h == 0 {
		return true
	}
	child := uint32(WS_CHILD | WS_VISIBLE)
	mk := func(ex uint32, class, text string, style uint32, id int, x, y, w, hh int) uintptr {
		c := createWindow(ex, class, text, child|style, a.scale(x), a.scale(y), a.scale(w), a.scale(hh), h, uintptr(id))
		sendMessage(c, WM_SETFONT, a.font, 1)
		return c
	}
	head := fmt.Sprintf(T("%d Problem(e) gefunden, davon %d Hinweis(e). Doppelklick oder „Hingehen“ zeigt die Stelle."), len(issues), len(issues)-nErr)
	if beforeSave {
		head = T("Vor dem Speichern:") + " " + head
	}
	mk(0, "STATIC", head, SS_LEFT, 0, 12, 10, 590, 36)
	lst := mk(WS_EX_CLIENTEDGE, "LISTBOX", "", WS_VSCROLL|WS_HSCROLL|LBS_NOTIFY|LBS_NOINTEGRALHEIGHT|WS_TABSTOP, idMcList, 12, 50, 590, 300)
	const LB_SETHORIZONTALEXTENT = 0x0194
	sendMessage(lst, LB_SETHORIZONTALEXTENT, uintptr(a.scale(1600)), 0)
	for _, is := range issues {
		pre := "✕ "
		if !is.err {
			pre = "ⓘ "
		}
		sendMessage(lst, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(pre+is.text))))
	}
	sendMessage(lst, LB_SETCURSEL, 0, 0)
	y := 362
	mk(0, "BUTTON", T("Hingehen"), BS_PUSHBUTTON|WS_TABSTOP, idMcGo, 12, y, 130, 30)
	if beforeSave {
		mk(0, "BUTTON", T("Trotzdem speichern"), BS_PUSHBUTTON|WS_TABSTOP, idMcSave, 336, y, 150, 30)
		mk(0, "BUTTON", T("Abbrechen"), BS_DEFPUSH|WS_TABSTOP, idMcClose, 492, y, 110, 30)
	} else {
		mk(0, "BUTTON", T("Schließen"), BS_DEFPUSH|WS_TABSTOP, idMcClose, 492, y, 110, 30)
	}
	var wr, cr RECT
	pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&wr)))
	pGetClientRect.Call(h, uintptr(unsafe.Pointer(&cr)))
	frame := int(wr.Bottom-wr.Top) - int(cr.Bottom)
	pSetWindowPos.Call(h, 0, uintptr(wr.Left), uintptr(wr.Top), uintptr(W), uintptr(a.scale(y+42)+frame), 0x0004)
	pEnableWindow.Call(a.hwnd, 0)
	mcState.done, mcState.result = false, idMcClose
	var msg MSG
	for !mcState.done {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		if msg.Message == WM_KEYDOWN && msg.WParam == VK_ESCAPE {
			mcState.done, mcState.result = true, idMcClose
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	sel := int(sendMessage(lst, LB_GETCURSEL, 0, 0))
	pEnableWindow.Call(a.hwnd, 1)
	pDestroyWindow.Call(h)
	pSetFocus.Call(a.glHwnd)
	switch mcState.result {
	case idMcSave:
		return true
	case idMcGo:
		if sel >= 0 && sel < len(issues) && issues[sel].jump != nil {
			issues[sel].jump()
		} else if sel >= 0 && sel < len(issues) {
			a.flash(T("Für diesen Hinweis gibt es keine Stelle zum Hingehen."))
		}
	}
	return false
}

// layoutSave ordnet "Mod pruefen" und "Speichern" in einer Zeile an.
func (a *app) layoutSave(x, y, w, h int) {
	cw := w * 32 / 100
	moveWindow(a.hCheck, x, y, cw, h)
	moveWindow(a.hSave, x+cw+a.scale(4), y, w-cw-a.scale(4), h)
}
