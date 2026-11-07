//go:build windows

package main

// "NPCs entfernen": alle (oder alle in einem Umkreis) NPCs der Welt in der Mod
// loeschen, z. B. als Ausgangspunkt fuer eine Mod mit eigenen Figuren.
// Geloeschte NPCs bleiben ueber die Originalwelt als Vorbilder verfuegbar.

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"g3viewer/g3"
)

// Figuren, ohne die sich die Hauptgeschichte nicht beenden laesst.
var storyNPCs = map[string]bool{
	"xardas": true, "zuben": true, "rhobar": true, "diego": true, "gorn": true, "milten": true, "lester": true,
}

func isHero(name string) bool { return strings.HasPrefix(strings.ToLower(name), "pc_hero") }

// npcGroup: 0 = Mensch, 1 = Ork, 2 = Tier/Monster
func npcGroup(e *g3.Entity) int {
	switch strings.ToLower(e.Mesh) {
	case "g3_hero_skeleton.fxa":
		return 0
	case "g3_orc_skeleton.fxa":
		return 1
	}
	return 2
}

// --- Vorbilder aus der Originalwelt ------------------------------------------

type baseRef struct {
	inst   int32 // Instanz in der Welt (-1 = nur in der Originalwelt)
	name   string
	sector string
	file   *g3.WorldFile // bei inst < 0: Originaldatei
	idx    int
}

// baseFile liefert Datei und Index eines Vorbilds.
func (a *app) baseFile(r *baseRef) (*g3.WorldFile, int, error) {
	if r.inst >= 0 {
		in := &a.world.Inst[r.inst]
		wf, err := a.editFile(in)
		if err != nil {
			return nil, 0, err
		}
		return wf, in.Ent.Index, nil
	}
	return r.file, r.idx, nil
}

// isModArchive: Archiv gehoert zur bearbeiteten Mod (nicht zum Spiel).
func (a *app) isModArchive(ar *g3.Pak) bool {
	for _, p := range a.mdsPakFiles() {
		if strings.EqualFold(p, ar.Path) {
			return true
		}
	}
	if m := loadConfig()["ModFile"]; m != "" && strings.EqualFold(filepath.Base(ar.Path), m) {
		return true
	}
	return false
}

// originalWorldFile liest eine Weltdatei so, wie sie im Spiel (ohne Mod) ist.
func (a *app) originalWorldFile(path string) (*g3.WorldFile, error) {
	if wf := a.origFiles[path]; wf != nil {
		return wf, nil
	}
	for i := len(a.vfs.Archives) - 1; i >= 0; i-- {
		ar := a.vfs.Archives[i]
		if a.isModArchive(ar) {
			continue
		}
		if !strings.EqualFold(strings.TrimSuffix(filepath.Base(ar.Path), filepath.Ext(ar.Path)), "Projects_compiled") {
			continue
		}
		for _, e := range ar.Entries {
			if !strings.EqualFold(e.Name, path) {
				continue
			}
			d, err := e.Read()
			if err != nil {
				return nil, err
			}
			wf, err := g3.OpenWorldFile(d, path)
			if err != nil {
				return nil, err
			}
			if a.origFiles == nil {
				a.origFiles = map[string]*g3.WorldFile{}
			}
			a.origFiles[path] = wf
			return wf, nil
		}
	}
	return nil, fmt.Errorf(T("Datei %s nicht gefunden"), path)
}

// removedBases liefert menschliche NPCs der Originalwelt, die in der Mod
// (gespeichert oder noch nicht gespeichert) geloescht sind.
func (a *app) removedBases() []baseRef {
	// Ergebnis merken, solange sich an geloeschten Objekten nichts aendert
	sig := len(a.world.Inst)
	for _, wf := range a.ed.files {
		for _, d := range wf.Deleted {
			if d {
				sig += 1000003
			}
		}
	}
	if a.rbCache != nil && a.rbSig == sig {
		return a.rbCache
	}
	defer func() { a.rbSig = sig }()
	setText(a.hStatus, T("Gelöschte NPCs werden in der Originalwelt gesucht …"))
	a.lastStatusText = ""
	paths := map[string]bool{}
	for _, ar := range a.vfs.Archives {
		if !a.isModArchive(ar) {
			continue
		}
		for _, e := range ar.Entries {
			if strings.HasSuffix(strings.ToLower(e.Name), ".lrentdat") {
				paths[e.Name] = true
			}
		}
	}
	for p, wf := range a.ed.files {
		for _, d := range wf.Deleted {
			if d {
				paths[p] = true
				break
			}
		}
	}
	if len(paths) == 0 {
		a.rbCache = []baseRef{}
		return a.rbCache
	}
	visible := map[[16]byte]bool{}
	for i := range a.world.Inst {
		if in := &a.world.Inst[i]; !in.Hidden && in.Ent.Kind == g3.KindNPC {
			visible[in.Ent.GUID] = true
		}
	}
	var out []baseRef
	for p := range paths {
		wf, err := a.originalWorldFile(p)
		if err != nil {
			continue
		}
		sector := strings.TrimPrefix(p[:strings.LastIndex(p, "/")], "G3_World_01/")
		for i, e := range wf.Ents {
			if e.Kind != g3.KindNPC || e.Name == "" || !strings.EqualFold(e.Mesh, "G3_Hero_Skeleton.FXA") || isHero(e.Name) || visible[e.GUID] {
				continue
			}
			out = append(out, baseRef{inst: -1, name: e.Name, sector: sector, file: wf, idx: i})
		}
	}
	if out == nil {
		out = []baseRef{}
	}
	a.rbCache = out
	return out
}

// --- NPCs entfernen ----------------------------------------------------------

type clearOpts struct {
	humans, animals, keepStory, area bool
	radius                           float64 // Meter
}

var clearDlg struct {
	done, ok                          bool
	humans, animals, story, area, rad uintptr
}

var clearClassDone bool

const (
	idClrOK = 1 + iota
	idClrCancel
	idClrHumans
	idClrAnimals
	idClrStory
	idClrWorld
	idClrArea
	idClrRadius
)

func clearWndProc(hwnd uintptr, msg uint32, w, l uintptr) uintptr {
	switch msg {
	case WM_COMMAND:
		switch loword(w) {
		case idClrOK:
			clearDlg.ok = true
			clearDlg.done = true
			return 0
		case idClrCancel:
			clearDlg.done = true
			return 0
		}
	case WM_CLOSE:
		clearDlg.done = true
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), w, l)
	return r
}

// askClearOpts zeigt das Fenster mit den Einstellungen.
func (a *app) askClearOpts() (clearOpts, bool) {
	var o clearOpts
	if !clearClassDone {
		if err := registerClass("AdanosEditClear", syscall.NewCallback(clearWndProc), 0, 16); err != nil {
			return o, false
		}
		clearClassDone = true
	}
	W, H := a.scale(580), a.scale(360)
	var rc RECT
	pGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	pt := POINT{int32((int(rc.Right) - W) / 2), int32((int(rc.Bottom) - H) / 2)}
	pClientToScreen.Call(a.hwnd, uintptr(unsafe.Pointer(&pt)))
	h := createWindow(WS_EX_DLGMODAL, "AdanosEditClear", T("NPCs entfernen"), WS_POPUP|WS_CAPTION|WS_SYSMENU|WS_VISIBLE,
		int(pt.X), int(pt.Y), W, H, a.hwnd, 0)
	if h == 0 {
		return o, false
	}
	child := uint32(WS_CHILD | WS_VISIBLE)
	mk := func(ex uint32, class, text string, style uint32, id int, x, y, w, hh int) uintptr {
		c := createWindow(ex, class, text, child|style, a.scale(x), a.scale(y), a.scale(w), a.scale(hh), h, uintptr(id))
		sendMessage(c, WM_SETFONT, a.font, 1)
		return c
	}
	mk(0, "STATIC", T("Entfernt NPCs aus der Welt deiner Mod. Das Originalspiel bleibt unverändert; Strg+Z holt alle auf einmal zurück. Der Held wird nie entfernt."), SS_LEFT, 0, 12, 10, 545, 72)
	clearDlg.humans = mk(0, "BUTTON", T("Menschen und Orks"), BS_AUTOCHECKBOX|WS_TABSTOP, idClrHumans, 12, 88, 545, 24)
	clearDlg.animals = mk(0, "BUTTON", T("Tiere und Monster"), BS_AUTOCHECKBOX|WS_TABSTOP, idClrAnimals, 12, 114, 545, 24)
	clearDlg.story = mk(0, "BUTTON", T("Story-Figuren behalten (Xardas, Zuben, Rhobar, Diego, Gorn, Milten, Lester)"), BS_AUTOCHECKBOX|WS_TABSTOP, idClrStory, 12, 140, 545, 24)
	world := mk(0, "BUTTON", T("In der ganzen Welt"), BS_AUTORADIOBUTTON|WS_GROUP|WS_TABSTOP, idClrWorld, 12, 176, 545, 24)
	clearDlg.area = mk(0, "BUTTON", T("Nur im Umkreis um die Kamera (Meter):"), BS_AUTORADIOBUTTON, idClrArea, 12, 202, 360, 24)
	clearDlg.rad = mk(WS_EX_CLIENTEDGE, "EDIT", "100", ES_AUTOHSCROLL|WS_TABSTOP, idClrRadius, 380, 202, 80, 24)
	mk(0, "BUTTON", T("NPCs entfernen"), BS_DEFPUSH|WS_TABSTOP, idClrOK, 290, 262, 160, 30)
	mk(0, "BUTTON", T("Abbrechen"), BS_PUSHBUTTON|WS_TABSTOP, idClrCancel, 456, 262, 100, 30)
	setChecked(clearDlg.humans, true)
	setChecked(clearDlg.story, true)
	setChecked(world, true)
	clearDlg.done, clearDlg.ok = false, false
	pEnableWindow.Call(a.hwnd, 0)
	var msg MSG
	for !clearDlg.done {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		if msg.Message == WM_KEYDOWN && msg.WParam == VK_ESCAPE {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	o.humans = isChecked(clearDlg.humans)
	o.animals = isChecked(clearDlg.animals)
	o.keepStory = isChecked(clearDlg.story)
	o.area = isChecked(clearDlg.area)
	o.radius, _ = strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(getText(clearDlg.rad)), ",", "."), 64)
	ok := clearDlg.ok
	pEnableWindow.Call(a.hwnd, 1)
	pDestroyWindow.Call(h)
	pSetFocus.Call(a.glHwnd)
	return o, ok
}

// clearNPCs fragt die Einstellungen ab und entfernt die NPCs.
func (a *app) clearNPCs() {
	if a.world == nil {
		return
	}
	o, ok := a.askClearOpts()
	if !ok {
		return
	}
	if !o.humans && !o.animals {
		a.flash(T("Nichts ausgewählt."))
		return
	}
	if o.area && o.radius <= 0 {
		messageBox(a.hwnd, T("Bitte einen Umkreis in Metern angeben."), T(appTitleDE), MB_ICONWARNING)
		return
	}
	r2 := float32(o.radius*100) * float32(o.radius*100) // Gothic 3 rechnet in Zentimetern
	cam := a.cam.Pos
	var list []int32
	for i := range a.world.Inst {
		in := &a.world.Inst[i]
		e := in.Ent
		if e.Kind != g3.KindNPC || in.Hidden || isHero(e.Name) || !strings.HasSuffix(strings.ToLower(e.Source), ".lrentdat") {
			continue
		}
		if o.keepStory && storyNPCs[strings.ToLower(e.Name)] {
			continue
		}
		g := npcGroup(e)
		if g < 2 && !o.humans || g == 2 && !o.animals {
			continue
		}
		if o.area {
			dx, dz := e.World[12]-cam[0], e.World[14]-cam[2]
			if dx*dx+dz*dz > r2 {
				continue
			}
		}
		list = append(list, int32(i))
	}
	if len(list) == 0 {
		a.flash(T("Keine passenden NPCs gefunden."))
		return
	}
	if messageBoxRet(a.hwnd, fmt.Sprintf(T("%d NPCs werden aus der Welt deiner Mod entfernt.\n\nQuests und Dialoge dieser NPCs sind dann im Spiel nicht mehr erreichbar. Fortfahren?"), len(list)), T(appTitleDE), MB_YESNO|MB_ICONWARNING) != IDYES {
		return
	}
	type del struct {
		wf *g3.WorldFile
		k  int
	}
	var done []del
	files := map[*g3.WorldFile]bool{}
	failed := 0
	for _, i := range list {
		in := &a.world.Inst[i]
		wf, err := a.editFile(in)
		if err != nil || wf.Deleted[in.Ent.Index] {
			failed++
			continue
		}
		if err := wf.Delete(in.Ent.Index); err != nil {
			failed++
			continue
		}
		done = append(done, del{wf, in.Ent.Index})
		files[wf] = true
	}
	for wf := range files {
		a.syncFile(wf)
	}
	a.ed.undo = append(a.ed.undo, func() {
		for _, d := range done {
			d.wf.Undelete(d.k)
		}
		for wf := range files {
			a.syncFile(wf)
		}
	})
	a.select_(-1, false)
	msg := fmt.Sprintf(T("%d NPCs entfernt. Strg+Z holt sie zurück; speichern nicht vergessen. Im Reiter NPCs stehen sie weiter als Vorbilder zur Verfügung."), len(done))
	if failed > 0 {
		msg += " " + fmt.Sprintf(T("%d konnten nicht entfernt werden."), failed)
	}
	a.flash(msg)
	if a.tab == 1 {
		a.searchNPCBases()
	}
}
