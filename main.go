//go:build windows

// AdanosEdit (frueher G3 Viewer): Mod-Editor fuer Gothic 3.
package main

import (
	"fmt"
	"g3viewer/lang"
	"image"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"g3viewer/g3"
	"g3viewer/scene"
)

const appTitleDE = "AdanosEdit – Gothic 3 Mod-Editor"
const version = "1.3"

// Steuerelement-IDs
const (
	idSearch = 101 + iota
	idResults
	idChkNPC
	idChkItems
	idChkTrees
	idChkLights
	idChkHelpers
	idChkWater
	idChkBarrier
	idChkTextures
	idChkWire
	idDistMinus
	idDistPlus
	idInfo
	idFolder
	idHelp
	idEdX
	idEdY
	idEdZ
	idEdRot
	idApply
	idCopy
	idDelete
	idUndo
	idClearNPC
	idSave
	idModeWorld
	idModeTpl
	idPlace
	idLang
	idMod
	idChest
)

var chkLabels = map[int]string{idChkNPC: "Figuren (NPCs und Tiere)", idChkItems: "Gegenstände (Platzhalter)",
	idChkTrees: "Bäume und Büsche", idChkLights: "Lichtquellen", idChkHelpers: "Hilfsobjekte (Wegpunkte, Zonen)",
	idChkWater: "Wasser", idChkBarrier: "Magische Barriere (Vengard)",
	idChkTextures: "Texturen", idChkWire: "Drahtgitter"}

type app struct {
	lastShot                          *image.RGBA
	hModeWorld, hModeTpl, hPlace      uintptr
	hLangLbl, hLang                   uintptr
	hFlag, flagBmp                    uintptr // Landesflagge neben "Sprache:"
	modRename                         bool    // Auswahlfeld "Mod" enthaelt "Mod umbenennen …"
	hTabWorld, hTabNPC, hTabDlg       uintptr
	hTabQst                           uintptr
	qst                               qstUI
	dlg                               dlgUI
	itemSet                           map[string]bool
	guidInst                          map[[16]byte]int32
	guidN                             int
	rbCache                           []baseRef
	rbSig                             int
	tab                               int
	npc                               npcUI
	prev                              *preview
	hModLbl, hMod                     uintptr
	modList                           []modInfo
	modSel                            int
	clip                              *clipEntry
	tplMode                           bool
	templates                         []tplEntry
	tplResults                        []int
	tplCh                             chan []tplEntry
	shotReq                           bool
	hwnd, glHwnd                      uintptr
	hSearchLbl, hSearch, hResults     uintptr
	hChk                              map[int]uintptr
	showWater, showBarrier            bool
	zoneLoop                          []scene.Vec3 // Umriss der gewaehlten Zone (Welt)
	zoneTop, zoneBottom               float32
	hDistLbl, hDistMinus, hDistPlus   uintptr
	hInfoLbl, hInfo, hStatus, hFolder uintptr
	hHelp                             uintptr
	font                              uintptr
	dpi                               float32
	hdc, hglrc                        uintptr
	width, height                     int

	gothicDir string
	vfs       *g3.VFS
	world     *scene.World
	worldCh   chan *scene.World
	progress  chan string
	loader    *scene.Loader
	ren       *renderer

	cam      scene.Camera
	speed    float32
	viewDist float32
	show     map[scene.MarkerKind]bool

	results  []int32
	selected int32

	// Maus
	dragging   bool
	dragButton int
	dragStart  POINT
	dragMoved  bool
	pickAt     *POINT

	lastStream, lastStatus, lastUnload time.Time
	startTime                          time.Time
	frames                             int
	fps                                float64
	statusText, lastStatusText         string
	ed                                 *editor
	hEditLbl, hEdX, hEdY, hEdZ, hEdRot uintptr
	hLblX, hLblY, hLblZ, hLblRot       uintptr
	hApply, hCopy, hDelete, hUndo      uintptr
	hClearNPC, hChest                  uintptr
	hGotoLbl, hGoto, hGotoMove         uintptr
	panelW                             [4]int // Breite der linken Leiste je Reiter (0 = Vorgabe)
	splitDrag                          bool
	ac                                 autoComplete
	origFiles                          map[string]*g3.WorldFile
	hSave                              uintptr
	hCheck                             uintptr // Mod pruefen
	flashText                          string
	flashUntil                         time.Time
	lastCenter                         scene.Vec3
	centerValid                        bool
	moving                             bool
	moveOld                            [16]float32
	moveOffset                         scene.Vec3
	moveY                              float32
	clickPos                           POINT
	multi                              map[int32]bool // Mehrfachauswahl (inkl. a.selected), nil = nur ein Objekt
	pickCtrl                           bool           // Klick mit Strg: zur Auswahl hinzufuegen/entfernen
	boxing                             bool           // Strg + Ziehen: Auswahlrahmen
	boxRect                            [4]int32
	boxReq                             *[4]int32
	moveGroup                          []groupMember // beim Verschieben mit Alt: alle Objekte der Auswahl
	moveGroupOld                       [][16]float32
	loadErrors                         int
	camPlaced                          bool
}

var theApp *app

func (a *app) scale(v int) int { return int(float32(v) * a.dpi) }

func main() {
	runtime.LockOSThread()
	// nur einmal starten: zwei Fenster wuerden sich beim Speichern gegenseitig stoeren
	h, _, err := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(u16("AdanosEdit_EinzigeInstanz"))))
	instanceMutex = h
	if h != 0 && err == syscall.Errno(183) {
		lang.SetCode(loadConfig()["Language"])
		if w, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(u16("AdanosEditMain"))), 0); w != 0 {
			pShowWindow.Call(w, 9) // SW_RESTORE
			pSetForegroundWindow.Call(w)
		}
		messageBox(0, T("AdanosEdit läuft bereits. Bitte das offene Fenster verwenden."), T(appTitleDE), MB_ICONINFORMATION)
		return
	}
	setupLog()
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 8192)
			n := runtime.Stack(buf, false)
			log.Printf("Absturz: %v\n%s", r, buf[:n])
			messageBox(0, fmt.Sprintf(T("Unerwarteter Fehler:\n%v\n\nDetails stehen in AdanosEdit.log."), r), T(appTitleDE), MB_ICONERROR)
		}
	}()
	pSetProcessDPIAware.Call()
	enableVisualStyles()
	icc := INITCOMMONCONTROLSEX{DwSize: uint32(unsafe.Sizeof(INITCOMMONCONTROLSEX{})), DwICC: ICC_STANDARD_CLASSES | ICC_WIN95_CLASSES}
	pInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&icc)))

	a := &app{selected: -1, speed: 1500, viewDist: 40000, dpi: 1,
		show:    map[scene.MarkerKind]bool{scene.MarkNPC: true, scene.MarkItem: true, scene.MarkAnimated: true, scene.MarkTree: true},
		worldCh: make(chan *scene.World, 1), progress: make(chan string, 16), tplCh: make(chan []tplEntry, 1), startTime: time.Now()}
	theApp = a
	a.ed = newEditor()
	cfg := loadConfig()
	lang.SetCode(cfg["Language"])
	if v, ok := cfg["ViewDist"]; ok {
		var d float32
		if _, err := fmt.Sscan(v, &d); err == nil && d >= 5000 && d <= 300000 {
			a.viewDist = d
		}
	}
	a.cam = scene.Camera{Pos: scene.Vec3{88600, 7600, -16500}, FovY: 60}
	if v := cfg["CamPos"]; v != "" {
		// Kamera nach einem Neustart (Mod-Wechsel) wiederherstellen
		var x, y, z, yaw, pitch float32
		if n, _ := fmt.Sscan(v, &x, &y, &z, &yaw, &pitch); n == 5 {
			a.cam.Pos, a.cam.Yaw, a.cam.Pitch = scene.Vec3{x, y, z}, yaw, pitch
		}
		delete(cfg, "CamPos")
		saveConfig(cfg)
	}
	a.cam.LookAt(scene.Vec3{90500, 5200, -11500})

	if err := a.createWindows(); err != nil {
		messageBox(0, err.Error(), T(appTitleDE), MB_ICONERROR)
		return
	}
	if err := a.initGL(); err != nil {
		log.Println("OpenGL:", err)
		messageBox(a.hwnd, fmt.Sprintf(T("OpenGL konnte nicht gestartet werden:\n%s"), err.Error()), T(appTitleDE), MB_ICONERROR)
		return
	}
	a.ren = newRenderer()

	a.gothicDir = findGothicDir(cfg["GothicDir"])
	if a.gothicDir == "" {
		messageBox(a.hwnd, T("Bitte wähle im nächsten Fenster deinen Gothic-3-Ordner aus (der Ordner, in dem Gothic3.exe und der Ordner Data liegen)."), T(appTitleDE), MB_ICONINFORMATION)
		for {
			d := browseFolder(a.hwnd, T("Gothic-3-Ordner auswählen"))
			if d == "" {
				return
			}
			if validGothicDir(d) {
				a.gothicDir = d
				break
			}
			messageBox(a.hwnd, T("In diesem Ordner wurde kein Data\\Projects_compiled.pak gefunden. Bitte den Gothic-3-Hauptordner wählen."), T(appTitleDE), MB_ICONERROR)
		}
	}
	cfg["GothicDir"] = a.gothicDir
	saveConfig(cfg)
	if err := a.openGame(); err != nil {
		messageBox(a.hwnd, fmt.Sprintf(T("Die Spieldaten konnten nicht geöffnet werden:\n%s"), err.Error()), T(appTitleDE), MB_ICONERROR)
		return
	}
	a.loop()
	cfg = loadConfig() // waehrend der Sitzung gespeicherte Werte behalten
	cfg["ViewDist"] = fmt.Sprintf("%.0f", a.viewDist)
	saveConfig(cfg)
	a.shutdown()
}

// shutdown gibt den OpenGL-Kontext frei und beendet den Prozess sofort.
// Manche Grafiktreiber haengen sonst beim Beenden mit aktivem Kontext.
func (a *app) shutdown() {
	if a.loader != nil {
		a.loader.Close()
	}
	if a.hglrc != 0 {
		pwglMakeCurrent.Call(0, 0)
		pwglDeleteContext.Call(a.hglrc)
	}
	log.Println("Beendet")
	os.Exit(0)
}

func (a *app) openGame() error {
	a.checkCommunityPatch()
	a.checkOrphanStrings()
	v, err := g3.OpenVFS(filepath.Join(a.gothicDir, "Data"))
	if err != nil {
		return err
	}
	for _, w := range v.Warnings {
		log.Println("Archiv:", w)
	}
	if mds := a.findMDS(); mds != "" {
		os.MkdirAll(filepath.Join(mds, "FanMods"), 0755) // fehlt beim CP-Mod-Starter im Download
		if n := a.syncModExt(); n > 0 {
			messageBox(a.hwnd, fmt.Sprintf(T("Deine AdanosEdit-Mods wurden an den Mod Starter in\n%s\nangepasst (Dateiendung jetzt %s).\n\nBitte die Mod dort einmal neu integrieren."), mds, a.modExt()), T(appTitleDE), MB_ICONINFORMATION)
		}
	}
	a.dropOtherMods(v)
	// Eigene Mod-Starter-Mod als oberste Schicht laden, damit man daran weiterarbeitet
	for _, p := range a.mdsPakFiles() {
		if err := v.Reopen(p, nil); err != nil {
			log.Println("Mod-Starter-Mod:", err)
		}
	}
	var names []string
	for _, ar := range v.Archives {
		names = append(names, filepath.Base(ar.Path))
	}
	log.Println("Gothic-Ordner:", a.gothicDir)
	log.Println("Archive:", strings.Join(names, ", "))
	a.vfs = v
	a.fillModList()
	a.updateTitle()
	runExtract(v)
	a.loadTemplates()
	a.loader = scene.NewLoader(v, max(2, min(runtime.NumCPU()-1, 6)))
	// Diagnose: einzelne Spieldateien neben das Programm kopieren (DumpMesh=name1;name2)
	if dm := loadConfig()["DumpMesh"]; dm != "" {
		for _, n := range strings.Split(dm, ";") {
			n = strings.TrimSpace(n)
			for _, cat := range []string{"_compiledMesh", "_compiledMaterial", "_compiledImage"} {
				if e := v.FindBase(cat, n); e != nil {
					if d, err := e.Read(); err == nil {
						os.WriteFile(filepath.Join(exeDir(), "dump_"+n), d, 0644)
						log.Println("Diagnose: kopiert", n)
					}
				}
			}
		}
	}
	var nSpecial int32
	a.loader.SpecialLog = func(s string) {
		if atomic.AddInt32(&nSpecial, 1) <= 40 {
			log.Println("Wasser/Barriere:", s)
		}
	}
	go func() {
		t := time.Now()
		w := scene.BuildWorld(v, func(done, total int) {
			select {
			case a.progress <- fmt.Sprintf(T("Welt wird eingelesen … %d %%"), done*100/max(total, 1)):
			default:
			}
		})
		log.Printf("Welt eingelesen: %d Objekte aus %d Dateien in %v", len(w.Inst), w.Files, time.Since(t))
		for _, e := range w.Errors {
			log.Println("Weltdatei:", e)
		}
		a.worldCh <- w
	}()
	return nil
}

func (a *app) loop() {
	var msg MSG
	last := time.Now()
	for {
		for {
			r, _, _ := pPeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, PM_REMOVE)
			if r == 0 {
				break
			}
			if msg.Message == WM_QUIT {
				if a.loader != nil {
					a.loader.Close()
				}
				return
			}
			if a.acKey(&msg) {
				continue
			}
			pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}
		a.acTick()
		now := time.Now()
		dt := float32(now.Sub(last).Seconds())
		last = now
		if dt > 0.1 {
			dt = 0.1
		}
		if ic, _, _ := pIsIconic.Call(a.hwnd); ic != 0 {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		a.frame(dt, now)
	}
}

func (a *app) frame(dt float32, now time.Time) {
	select {
	case l := <-a.tplCh:
		a.templates = l
		if a.tab == 1 {
			a.fillNPCCombos()
		}
		if a.tplMode {
			a.onSearch()
		}
	case w := <-a.worldCh:
		a.world = w
		a.clearMulti()
		a.statusText = ""
		a.onSearch()
		if a.tab == 2 {
			a.fillDlgNPCs()
		}
		a.ensureStringtableMod()
	default:
	}
	for {
		select {
		case s := <-a.progress:
			a.statusText = s
			continue
		default:
		}
		break
	}
	a.move(dt)
	secs := now.Sub(a.startTime).Seconds()
	if a.world != nil && now.Sub(a.lastStream) > 250*time.Millisecond {
		a.lastStream = now
		a.stream(secs)
	}
	// Ergebnisse des Laders hochladen (Zeitbudget pro Frame)
	budget := time.Now().Add(12 * time.Millisecond)
	for time.Now().Before(budget) {
		select {
		case r := <-a.loader.Out:
			if r.Tex != nil {
				if r.Tex.Err != "" {
					log.Printf("Textur %s: %s", r.Tex.Key, r.Tex.Err)
				}
				a.ren.uploadTexture(r.Tex)
			} else if r.Model != nil {
				if r.Model.Err != "" {
					a.loadErrors++
					if a.loadErrors <= 300 {
						log.Println("Modell:", r.Model.Err)
					}
				}
				a.ren.uploadModel(r.Model, secs)
			}
			continue
		default:
		}
		break
	}
	if now.Sub(a.lastUnload) > 2*time.Second {
		a.lastUnload = now
		a.unloadFar(secs)
	}
	if a.width <= 0 || a.height <= 0 {
		time.Sleep(10 * time.Millisecond)
		return
	}
	vs := &viewState{cam: a.cam, width: a.width, height: a.height, viewDist: a.viewDist, show: a.show, now: secs, water: a.showWater, barrier: a.showBarrier}
	vs.proj = scene.Perspective(a.cam.FovY, float32(a.width)/float32(a.height), 30, a.viewDist*1.05)
	vs.view = a.cam.View()
	a.ren.collect(a.world, vs)
	if a.pickAt != nil {
		var list []int32
		a.ren.draw(a.world, vs, true, &list)
		id := readPick(int(a.pickAt.X), int(a.pickAt.Y), a.height, list)
		hit, hitOK := readWorldAt(vs, int(a.pickAt.X), int(a.pickAt.Y))
		a.pickAt = nil
		if a.npc.pick > 0 {
			a.assignRoutinePoint(id, hit, hitOK)
		} else if a.pickCtrl {
			a.pickCtrl = false
			a.toggleMulti(id)
		} else {
			a.clearMulti()
			a.select_(id, false)
			if a.tab == 1 {
				a.fillNPCItems(id)
				a.updateNPCInfo()
			}
		}
	}
	if a.boxReq != nil {
		a.boxSelect(vs, *a.boxReq)
		a.boxReq = nil
	}
	a.ren.draw(a.world, vs, false, nil)
	if a.shotReq {
		a.shotReq = false
		a.grabShot()
	}
	// Weltpunkt in der Bildmitte (fuer "in der Bildmitte erstellen/platzieren")
	// in jedem Bild neu bestimmen, auch ohne Auswahl
	a.lastCenter, a.centerValid = readCenter(vs)
	if a.inGroup() && a.world != nil {
		for id := range a.multi {
			if id != a.selected && !a.world.Inst[id].Hidden {
				drawSelection(&a.world.Inst[id])
			}
		}
	}
	if a.boxing {
		drawBoxRect(a.boxRect, a.width, a.height)
	}
	if a.selected >= 0 && a.world != nil {
		drawSelection(&a.world.Inst[a.selected])
		if len(a.zoneLoop) > 1 {
			drawZoneOutline(a.zoneLoop, a.zoneTop, a.zoneBottom, markerColor(a.world.Inst[a.selected].Marker))
		}
	}
	a.drawPreview(secs)
	pSwapBuffers.Call(a.hdc)
	if wglSwapIntervalPtr == 0 {
		time.Sleep(4 * time.Millisecond)
	}
	a.frames++
	if now.Sub(a.lastStatus) > 400*time.Millisecond {
		el := now.Sub(a.lastStatus).Seconds()
		if !a.lastStatus.IsZero() {
			a.fps = float64(a.frames) / el
		}
		a.frames = 0
		a.lastStatus = now
		a.updateStatus()
	}
}

// stream legt fest, welche Modelle in Sichtweite geladen werden sollen.
func (a *app) stream(secs float64) {
	type want struct {
		key string
		d   float32
	}
	best := map[string]float32{}
	cam := a.cam.Pos
	f := a.cam.Forward()
	a.world.Query(cam, a.viewDist, func(i int32) {
		in := &a.world.Inst[i]
		if in.ModelKey == "" {
			return
		}
		d := in.Center.Sub(cam).Len() - in.Radius
		if d > a.viewDist {
			return
		}
		if d < 0 {
			d = 0
		}
		if m := a.ren.model(in); m != nil {
			m.lastUsed = secs
			return
		}
		// Objekte in Blickrichtung zuerst, kleine weit entfernte zuletzt
		dir := in.Center.Sub(cam).Norm()
		if dir.Dot(f) < 0.2 {
			d = d*1.6 + 2000
		}
		if in.Radius < 300 {
			d *= 1.5
		}
		if old, ok := best[in.ModelKey]; !ok || d < old {
			best[in.ModelKey] = d
		}
	})
	list := make([]want, 0, len(best))
	for k, d := range best {
		list = append(list, want{k, d})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].d < list[j].d })
	keys := make([]string, 0, len(list)+1)
	for _, k := range a.previewKeys() {
		if _, ok := a.ren.modelIndex[k]; !ok {
			keys = append(keys, k)
		}
	}
	for _, w := range list {
		keys = append(keys, w.key)
	}
	a.loader.Want(keys)
}

// unloadFar entlaedt lange nicht benutzte Modelle, wenn viel Grafikspeicher belegt ist.
func (a *app) unloadFar(secs float64) {
	const budget = 1400 << 20
	type cand struct {
		i    int32
		last float64
	}
	var c []cand
	for i, m := range a.ren.models {
		if m == nil || m.failed {
			continue
		}
		age := secs - m.lastUsed
		if age > 90 || (a.ren.gpuBytes > budget && age > 3) {
			c = append(c, cand{int32(i), m.lastUsed})
		}
	}
	sort.Slice(c, func(i, j int) bool { return c[i].last < c[j].last })
	for _, x := range c {
		if secs-x.last <= 90 && a.ren.gpuBytes <= budget*8/10 {
			break
		}
		key := a.ren.models[x.i].key
		for _, t := range a.ren.unloadModel(x.i) {
			a.loader.ForgetTex(t)
		}
		a.loader.Forget(key)
	}
}

func (a *app) move(dt float32) {
	if f, _, _ := pGetFocus.Call(); f != a.glHwnd {
		return
	}
	sp := a.speed
	if keyDown(VK_SHIFT) {
		sp *= 4
	}
	if keyDown(VK_CONTROL) {
		sp *= 0.25
	}
	var d scene.Vec3
	fw, rt := a.cam.Forward(), a.cam.Right()
	if keyDown('W') || keyDown(VK_UP) {
		d = d.Add(fw)
	}
	if keyDown('S') || keyDown(VK_DOWN) {
		d = d.Sub(fw)
	}
	if keyDown('D') || keyDown(VK_RIGHT) {
		d = d.Add(rt)
	}
	if keyDown('A') || keyDown(VK_LEFT) {
		d = d.Sub(rt)
	}
	if keyDown('Y') || keyDown(VK_SPACE) {
		d = d.Add(scene.Vec3{0, 1, 0})
	}
	if keyDown('X') {
		d = d.Sub(scene.Vec3{0, 1, 0})
	}
	if d.Len() > 0 {
		a.cam.Pos = a.cam.Pos.Add(d.Norm().Scale(sp * dt))
	}
}

// selectionLabel: Name des gewaehlten Objekts (mit Anzeigenamen im Spiel).
func (a *app) selectionLabel() string {
	if a.world == nil || a.selected < 0 || int(a.selected) >= len(a.world.Inst) {
		return ""
	}
	if a.inGroup() {
		return fmt.Sprintf(T("%d Objekte"), len(a.selection()))
	}
	in := &a.world.Inst[a.selected]
	if in.Hidden {
		return ""
	}
	name := in.Ent.Name
	if name == "" {
		name = in.Ent.Mesh
	}
	if d := a.displayName(in.Ent.Name); in.Ent.Name != "" && d != in.Ent.Name {
		return fmt.Sprintf(T("%s („%s“)"), name, d)
	}
	return name
}

func (a *app) updateStatus() {
	p := a.cam.Pos
	txt := a.statusText
	if txt == "" {
		pending := 0
		if a.loader != nil {
			pending = a.loader.Pending
		}
		n := 0
		for _, m := range a.ren.models {
			if m != nil {
				n++
			}
		}
		txt = fmt.Sprintf(T("Position %.0f / %.0f / %.0f   |   %.0f FPS   |   Modelle %d (wartend %d)   |   Grafikspeicher %d MB   |   Tempo %.0f   |   F1 = Hilfe"),
			p[0], p[1], p[2], a.fps, n, pending, a.ren.gpuBytes>>20, a.speed)
		if sl := a.selectionLabel(); sl != "" {
			// gewaehltes Objekt in jedem Reiter sichtbar (nach der Position)
			if i := strings.Index(txt, "   |   "); i >= 0 {
				txt = txt[:i] + "   |   " + fmt.Sprintf(T("Ausgewählt: %s"), sl) + txt[i:]
			}
		}
	}
	if a.flashText != "" && time.Now().Before(a.flashUntil) {
		txt = a.flashText
	}
	if txt != a.lastStatusText {
		a.lastStatusText = txt
		setText(a.hStatus, txt)
	}
}

func (a *app) setViewDist(d float32) {
	if d < 5000 {
		d = 5000
	}
	if d > 200000 {
		d = 200000
	}
	a.viewDist = d
	setText(a.hDistLbl, fmt.Sprintf(T("Sichtweite: %.0f m"), d/100))
}

// select_ waehlt ein Objekt aus (fly = Kamera hinbewegen).
func (a *app) select_(id int32, fly bool) {
	if id < 0 || (a.multi != nil && !a.multi[id]) {
		a.clearMulti()
	}
	a.selected = id
	a.updateZoneOutline()
	if id < 0 {
		a.prev = nil // Vorschau beim Abwaehlen ausblenden
	}
	if id < 0 || a.world == nil {
		setText(a.hInfo, "")
		a.fillFields()
		return
	}
	in := &a.world.Inst[id]
	e := in.Ent
	var b strings.Builder
	name := e.Name
	if name == "" {
		name = T("(ohne Namen)")
	}
	fmt.Fprintf(&b, "%s\r\n", name)
	switch in.Marker {
	case scene.MarkArena:
		b.WriteString(T("Art: Arena (Zone für Arenakämpfe)") + "\r\n")
	case scene.MarkZone:
		b.WriteString(T("Art: Zone (z. B. für „Ort erreichen“)") + "\r\n")
	case scene.MarkPoint:
		b.WriteString(T("Art: Routinepunkt / Platz für den Tagesablauf") + "\r\n")
	}
	b.WriteString("\r\n")
	p := e.Pos()
	fmt.Fprintf(&b, T("Position: %.1f / %.1f / %.1f\r\n"), p[0], p[1], p[2])
	sc := scene.Vec3{e.World[0], e.World[1], e.World[2]}.Len()
	yaw := math.Atan2(float64(e.World[8]), float64(e.World[10])) * 180 / math.Pi
	fmt.Fprintf(&b, T("Drehung (Y): %.1f°   Skalierung: %.2f\r\n"), yaw, sc)
	if e.Mesh != "" {
		fmt.Fprintf(&b, T("Modell: %s\r\n"), e.Mesh)
		if e.MatSwitch != 0 {
			fmt.Fprintf(&b, T("Materialvariante: %d\r\n"), e.MatSwitch)
		}
		if in.ModelKey != "" {
			if m := a.ren.model(in); m != nil && m.failed && m.err != "" {
				fmt.Fprintf(&b, T("Ladefehler: %s\r\n"), m.err)
			}
		}
	}
	b.WriteString(a.chestText(in))
	if e.Kind == g3.KindNPC {
		if wf, err := a.editFile(in); err == nil {
			b.WriteString(a.teachText(wf, e.Index))
		}
	}
	fmt.Fprintf(&b, "GUID: %s\r\n", e.GUIDString())
	fmt.Fprintf(&b, T("Sektor: %s\r\n"), in.Sector)
	fmt.Fprintf(&b, "\r\nProperty-Sets:\r\n")
	for _, c := range e.Classes {
		fmt.Fprintf(&b, "  %s\r\n", c)
	}
	if a.inGroup() {
		setText(a.hInfo, a.groupInfo())
	} else {
		setText(a.hInfo, b.String())
	}
	a.fillFields()
	if fly {
		d := in.Radius*2.2 + 400
		if d > 20000 {
			d = 20000
		}
		fw := a.cam.Forward()
		fw[1] = -0.35
		fw = fw.Norm()
		a.cam.Pos = in.Center.Sub(fw.Scale(d))
		a.cam.LookAt(in.Center)
	}
}

func (a *app) onSearch() {
	sendMessage(a.hResults, LB_RESETCONTENT, 0, 0)
	a.results = nil
	a.tplResults = nil
	txt := getText(a.hSearch)
	if a.tplMode {
		a.searchTemplates(txt)
		return
	}
	if a.world == nil {
		return
	}
	if len(strings.TrimSpace(txt)) < 2 {
		return
	}
	a.results = a.world.Search(txt, 1000)
	for _, i := range a.results {
		in := &a.world.Inst[i]
		sec := in.Sector
		if j := strings.LastIndex(sec, "/"); j >= 0 {
			sec = sec[j+1:]
		}
		s := in.Ent.Name
		if s == "" {
			s = in.Ent.Mesh
		}
		sendMessage(a.hResults, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(s+"   ("+sec+")"))))
	}
}

const helpText = `AdanosEdit – Gothic 3 Mod-Editor

Mit AdanosEdit veränderst du die Welt von Gothic 3, erstellst eigene NPCs, Dialoge und Quests und speicherst alles als Mod für den Gothic 3 Mod Starter. Die Originaldateien des Spiels bleiben dabei immer unverändert.


STEUERUNG IM 3D-FENSTER

Rechte (oder linke) Maustaste gedrückt halten + Maus bewegen: umsehen
W / A / S / D oder Pfeiltasten: bewegen
Y oder Leertaste: hoch,  X: runter
Umschalt: schneller,  Strg: langsamer
Mausrad: Grundtempo ändern
Linksklick: Objekt auswählen
F: zum ausgewählten Objekt fliegen
Esc: Auswahl aufheben (und Vorschau ausblenden)
Bild auf / Bild ab: Sichtweite ändern

Die Modelle in deiner Umgebung werden automatisch nachgeladen. Die linke Leiste lässt sich verbreitern: Maus an ihren rechten Rand (Doppelpfeil) und ziehen.

„Gehe zu:“ (unten in der Leiste) fliegt die Kamera in eine Stadt, ein Lager oder zum Spielstart. „Auswahl hierher“ versetzt das ausgewählte Objekt – z. B. einen NPC oder den Helden – an die Stelle in der Bildmitte (bzw. in die Ortsmitte, wenn die Kamera woanders hinschaut) und setzt es auf den Boden; bei NPCs können die Routinepunkte mitwandern.

Mit dem Häkchen „Hilfsobjekte“ erscheinen unsichtbare Spielobjekte als farbige Kästchen: grün = Zonen (z. B. für „Ort erreichen“), violett = Arenen, hellblau = Routinepunkte (Plätze für den Tagesablauf), grau = sonstige Hilfsobjekte. Wird eine Zone oder Arena angeklickt, zeigt AdanosEdit ihren Umriss.


REITER „WELT“: OBJEKTE BEARBEITEN

Alt + linke Maustaste ziehen: ausgewähltes Objekt waagerecht verschieben
Alt + Mausrad: Höhe ändern (mit Umschalt in 1-m-Schritten)
Umschalt+F: Objekt auf den Boden setzen
R / Umschalt+R: um 15° drehen (mit Strg: 1°)
Strg+D oder „Kopieren“: Kopie in der Bildmitte anlegen
Strg+C / Strg+V: Objekt kopieren und in der Bildmitte einfügen
Entf oder „Löschen“: Objekt löschen
Strg+Z: Rückgängig,  Strg+Y: Wiederherstellen,  Strg+S: als Mod speichern
Felder X/Y/Z/Drehung + „Übernehmen“: exakte Werte

Mehrere Objekte: Strg + Klick nimmt ein Objekt zur Auswahl hinzu oder wieder heraus, Strg + Ziehen wählt alle Objekte im Rahmen (sehr große wie Gelände nicht). Verschieben, Höhe, Drehen (um die gemeinsame Mitte), Umschalt+F und Entf wirken dann auf alle; Strg+Z macht es in einem Schritt rückgängig. Esc hebt die Auswahl auf.

Neue Objekte: „Neues Objekt aus Vorlage anlegen“ wählen, eine Vorlage suchen (z. B. „Trank“, „Truhe“, „Wolf“) und „Neu platzieren“ klicken oder doppelklicken. Ein Klick in der Liste zeigt oben links eine sich drehende Vorschau.

„NPCs entfernen …“ entfernt Menschen und Orks und auf Wunsch auch Tiere und Monster – in der ganzen Welt oder im Umkreis um die Kamera. Der Held wird nie entfernt, Story-Figuren (Xardas, Zuben, Rhobar, Diego, Gorn, Milten, Lester) bleiben auf Wunsch erhalten. Strg+Z holt alle auf einmal zurück.

„Truhe / Schloss …“: Truhe (oder Tür) anklicken und Inhalt sowie Schloss festlegen. Zufälliger Inhalt kommt aus Schatzsets (z. B. TS_Treasure_Small), feste Gegenstände liegen immer darin (z. B. „It_Gold 500; It_Apple 3“). Das Schloss kann offen, leicht/mittel/schwer zu knacken oder nur mit Schlüssel zu öffnen sein; als Schlüssel den Vorlagennamen eintragen (z. B. It_Key_Fabio). Gilt in einem neuen Spiel.


REITER „NPCS“: FIGUREN ERSTELLEN UND ÄNDERN

Neuer NPC: oben einen Vorbild-NPC suchen und anklicken. Auch gelöschte NPCs stehen als Vorbild zur Verfügung („gelöscht – nur als Vorbild“). Der neue NPC übernimmt Werte, Stimme und Kampfstil des Vorbilds. Dann Name, Kopf, Haare, Bart und Kleidung wählen (die Vorschau zeigt das Ergebnis) und „NPC in der Bildmitte erstellen“ klicken.

Gruppe: politische Ausrichtung (Orks, Rebellen, Nordmarer …) und Enklave (Stadt oder Lager, z. B. Montera).
Händler, Waffen, Beute, Taschen: Schatzsets – daraus erzeugt das Spiel Händlerwaren, Waffen, Beute nach dem Tod und was man stehlen kann.
Gegenst.: feste Gegenstände im Inventar, z. B. „It_Apple 3; It_Key_01 1“. Beim Tippen erscheint eine Vorschlagsliste (↑ ↓ wählen, Enter oder Tab übernehmen, Esc schließen); gesucht wird auch im deutschen Namen.
„↑ Auf gewählten NPC anwenden“ überträgt Gruppe, Händler, Waffen, Beute, Taschen und Gegenstände auf den in der Welt gewählten NPC. Steht etwas im Feld „Name“, wird es sein angezeigter Name.
„Händler / Lehrer …“ (neben „↑ Auf gewählten NPC anwenden“): legt für den in der Welt gewählten NPC fest, welche Attribute und Fähigkeiten er lehrt und ob Handel und Lehre von Anfang an offen sind. Sonst öffnet sie ein Dialog mit „> Handel an“ bzw. „> Lehren an“.

Tagesablauf: Arbeit, Freizeit und Schlaf. Gothic 3 schickt den NPC tagsüber zum Arbeitsplatz, abends zum Freizeitplatz und nachts zum Schlafplatz. Beim Erstellen werden dafür Routinepunkte neben dem NPC angelegt (sichtbar mit „Hilfsobjekte“). Bei vorhandenen NPCs: NPC anklicken, „Arbeit …“, „Freizeit …“ oder „Schlaf …“ wählen und in der Welt auf den neuen Platz klicken (Bank, Bett, Amboss, Routinepunkt …).

Weitere Tagesabläufe: Im Auswahlfeld über den Knöpfen „Neuer Tagesablauf …“ wählen, einen Namen vergeben (z. B. BeiJohn) und mit „Arbeit …“, „Freizeit …“, „Schlaf …“ die Plätze setzen. Ein Klick auf den Boden legt dort einen neuen Routinepunkt an. Der NPC wechselt erst in diesen Ablauf, wenn ein Dialog es befiehlt, z. B. im Abgabe-Dialog einer Quest: „> Tagesablauf Felix BeiJohn“. Löschen: den Ablauf im Auswahlfeld wählen und „Tagesablauf … löschen …“ (der Ablauf beim Spielstart bleibt immer erhalten).


REITER „DIALOGE“

NPC suchen und anklicken, dann einen Dialog wählen oder „Neuer Dialog“. » = der NPC spricht den Helden von selbst an, • = Auswahl im Gespräch. Auch Original-Dialoge lassen sich ändern; „Original wiederherstellen“ verwirft die Änderung.

Ablauf, eine Zeile pro Satz:
  Held: Was gibt es Neues?
  NPC: Nicht viel.
  NPC (SD_Angry): Und jetzt verschwinde!
Aktionen beginnen mit >:
  > Geben It_Gold 50   (NPC gibt dem Helden)
  > Nehmen It_Apple 2   (Held gibt dem NPC)
  > Erfahrung 100
  > Ereignis Name   (setzt ein Spielereignis)
  > Ende   (beendet das Gespräch)
  > Tagesablauf Felix BeiJohn   (NPC wechselt in einen anderen Tagesablauf)
  > Handel an / > Lehren an   (öffnet Handel bzw. Lehre beim NPC; „aus“ schließt sie wieder)
  > !Befehl;Wer;Wen;ID1;ID2;Text   (jeder Befehl aus G3IQ)

Bei „Auswahl im Gespräch“ ist der erste Satz des Helden der Text in der Auswahl.
Erst nach / Nicht nach: Der Dialog erscheint erst, wenn ein anderer Dialog (auch eines anderen NPCs) geführt wurde bzw. ein Spielereignis gesetzt ist – oder er verschwindet dann. Beim Tippen schlägt die Liste eigene Dialoge aller NPCs und alle Ereignisse des Spiels vor.
Quest / Wann: verbindet den Dialog mit einer Quest (vergeben, läuft, abgeben, erfolgreich, gescheitert).
Neue Sätze haben keine Sprachaufnahme und erscheinen als Untertitel; unveränderte Original-Sätze behalten ihre Stimme.


REITER „QUESTS“

„Neue Quest“, Titel (steht im Tagebuch), Aufgabe, Auftraggeber, Ziel, Anzahl, Erfahrung und Gold eintragen:
  Gegenstände bringen: Ziel = Gegenstand (z. B. It_Apple, mit Vorschlagsliste)
  Jemandem etwas ausrichten: Ziel = NPC, mit dem der Held reden soll
  Jemanden töten: Ziel = NPC oder Monster
  Jemanden besiegen: Ziel = NPC, den der Held im Zweikampf niederschlagen soll (nicht töten); ist er selbst der Auftraggeber, fordert er den Helden heraus
  Jemanden vertreiben: Ziel = NPC, der nach seiner Niederlage wegzieht (Tagesablauf „Vertrieben“ im Reiter NPCs anlegen)
  Arenakampf: Gegner + Arena (z. B. Montera_Arena)
  Ort erreichen: Ort = vorhandene Zone (z. B. Varant_Passarea) oder über „Zone wählen / anlegen …“ eine Zone in der Nähe bzw. eine neue runde Zone in der Bildmitte (wird beim Speichern in die Navigationskarte der Mod eingetragen), Wer = leer für den Helden
  Truhe ausrauben: Truhe anklicken und „Gewählte Truhe übernehmen“, Gegenstand und Anzahl eintragen
  Jemanden zu einer Person bringen: Wen bringen = NPC, der dem Helden folgt; Auftraggeber = Person, zu der er gebracht wird
  Jemandem folgen: Führer = NPC, der den Helden führt; Zielpunkt = Ort – Kamera hinbewegen und „Zielpunkt in der Bildmitte setzen“ klicken
  Zauber auf jemanden wirken: Auf wen = NPC, Zauber = welcher Zauber (z. B. Heilung anderer); die Schriftrolle gibt der Auftraggeber
Weitere Belohnungen: Ansehen bei einer Gruppe oder in einem Ort und ein Attribut. „Erst nach Quest“ macht eine Quest erst nach anderen verfügbar (Questketten), „Zeitlimit“ lässt den Auftrag nach so vielen Spieltagen verfallen.
„Passende Dialoge erzeugen“ legt die Gespräche zum Vergeben, Nachfragen und Abgeben an; im Reiter „Dialoge“ kannst du sie umschreiben. Erfahrung gibt das Spiel beim Erfolg, das Gold der Auftraggeber im Abschluss-Dialog.


MODS UND SPEICHERN

Über „Mod:“ wählst du, welche deiner Mods du bearbeitest. „Originalwelt (neue Mod anlegen)“ fragt gleich nach dem Namen der neuen Mod (Abbrechen = nur die Originalwelt ansehen); angelegt wird sie beim ersten Speichern. „Mod umbenennen …“ ändert den Namen der bearbeiteten Mod.

Gespeichert wird als Mod für den Mod Starter. Zum Spielen die Mod im Mod Starter anhaken und „Integrate mods“ klicken (vorher „Delete mods from game“). Ist die Mod schon integriert, aktualisiert AdanosEdit die Kopie im Spiel gleich mit; neu integrieren ist nur nötig, wenn ein neues Archiv dazukommt (z. B. die ersten Dialoge oder Quests) – darauf weist AdanosEdit nach dem Speichern hin.

Namen, Dialogsätze und Quest-Titel stehen zusätzlich in der stringtableMod.ini der Mod. Der Mod Starter trägt sie beim Integrieren in die Texttabelle ein – so kommen sie auch bei anderen Spielern an.

AdanosEdit arbeitet mit beiden verbreiteten Fassungen des Mod Starters: der aktuellen für den Community Patch (1.2.0.1, liegt meist in Gothic 3\Data\MDS, Mods als .m0x) und der älteren (Mods als .p0x). Welche verwendet wird, stellst du in der Mod-Auswahl oben unter „Mod Starter wählen …“ ein; die eigenen Mods werden dabei passend kopiert und umbenannt. Benutze im Spiel immer nur einen Mod Starter – vor einem Wechsel im bisherigen „Delete mods from game“ klicken.

Änderungen an der Welt am besten mit einem neuen Spiel prüfen; ältere Spielstände enthalten den alten Zustand der Welt.


HINWEISE

Figuren werden in der Grundhaltung gezeigt (ohne Animation). Bäume werden aus Rinde und Laub der Spieldaten nachgebildet und sehen deshalb nicht exakt wie im Spiel aus. Objekte ohne Modell erscheinen als farbige Platzhalter. Wasser und die magische Barriere über Vengard zeichnet das Spiel mit eigenen Effekten; AdanosEdit zeigt sie vereinfacht als durchscheinende Flächen (nicht anklickbar, abschaltbar über die Häkchen „Wasser“ und „Magische Barriere“).`

func (a *app) onCommand(id, code int) {
	if id == idNpcItems || id == idQstTarget || id == idQstPlace || id == idQstPrereq || id == idDlgCond || id == idDlgCondNot || id == idDlgNeed {
		h := map[int]uintptr{idNpcItems: a.npc.items, idQstTarget: a.qst.target, idQstPlace: a.qst.place, idQstPrereq: a.qst.prereq, idDlgCond: a.dlg.cond, idDlgCondNot: a.dlg.condNot, idDlgNeed: a.dlg.need}[id]
		if (id == idQstPlace || id == idQstPrereq) && code == EN_CHANGE && !a.qst.loading {
			a.qst.changed = true
		}
		if (id == idDlgCond || id == idDlgCondNot) && code == EN_CHANGE && !a.dlg.loading {
			a.dlg.changed = true
		}
		switch code {
		case EN_CHANGE:
			a.onACChange(h)
		case EN_KILLFOCUS:
			a.ac.hideAt = time.Now().Add(250 * time.Millisecond)
		}
	}
	switch id {
	case idSearch:
		if code == EN_CHANGE {
			a.onSearch()
		}
	case idResults:
		if a.tplMode {
			if te := a.selectedTemplate(); te != nil {
				if code == LBN_DBLCLK {
					a.placeTemplate()
				} else if code == LBN_SELCHANGE {
					a.select_(-1, false)
					a.showTemplateInfo(te)
					a.setPreviewTemplate(te.t)
				}
			}
			break
		}
		if code == LBN_SELCHANGE || code == LBN_DBLCLK {
			i := int(sendMessage(a.hResults, LB_GETCURSEL, 0, 0))
			if i >= 0 && i < len(a.results) {
				a.select_(a.results[i], true)
				a.setPreviewInstance(&a.world.Inst[a.results[i]])
			}
		}
	case idChkNPC:
		a.show[scene.MarkNPC] = isChecked(a.hChk[id])
		a.show[scene.MarkAnimated] = a.show[scene.MarkNPC]
	case idChkItems:
		a.show[scene.MarkItem] = isChecked(a.hChk[id])
	case idChkTrees:
		a.show[scene.MarkTree] = isChecked(a.hChk[id])
	case idChkLights:
		a.show[scene.MarkLight] = isChecked(a.hChk[id])
	case idChkHelpers:
		a.show[scene.MarkHelper] = isChecked(a.hChk[id])
		a.show[scene.MarkZone] = a.show[scene.MarkHelper]
		a.show[scene.MarkArena] = a.show[scene.MarkHelper]
		a.show[scene.MarkPoint] = a.show[scene.MarkHelper]
	case idChkWater:
		a.showWater = isChecked(a.hChk[id])
	case idChkBarrier:
		a.showBarrier = isChecked(a.hChk[id])
	case idChkTextures:
		a.ren.showTextures = isChecked(a.hChk[id])
	case idChkWire:
		a.ren.wireframe = isChecked(a.hChk[id])
	case idChest:
		a.editChest()
	case idNpcTeach:
		a.editTeach()
	case idDistMinus:
		a.setViewDist(a.viewDist / 1.25)
	case idDistPlus:
		a.setViewDist(a.viewDist * 1.25)
	case idHelp:
		a.showHelp()
	case idApply:
		a.applyFields()
	case idCopy:
		a.duplicateSelected()
	case idDelete:
		a.deleteSelected()
	case idClearNPC:
		a.clearNPCs()
	case idGoto:
		if code == CBN_SELCHANGE {
			a.gotoPlace()
		}
	case idGotoMove:
		a.moveSelectedToPlace()
	case idUndo:
		a.undoLast()
	case idModCheck:
		a.showModCheck(false)
	case idSave:
		a.save()
	case idModeWorld:
		a.setSearchMode(false)
	case idModeTpl:
		a.setSearchMode(true)
	case idPlace:
		a.placeTemplate()
	case idMod:
		if code == CBN_SELCHANGE {
			a.onModSelect()
		}
	case idTabWorld:
		a.setTab(0)
	case idTabNPC:
		a.setTab(1)
	case idTabDlg:
		a.setTab(2)
	case idDlgSearch:
		if code == EN_CHANGE {
			a.fillDlgNPCs()
		}
	case idDlgNPCs:
		if code == LBN_SELCHANGE {
			a.selectDlgNPC()
		}
	case idDlgList:
		if code == LBN_SELCHANGE {
			i := comboSelList(a.dlg.list)
			if i != a.dlg.cur {
				if a.commitDialog() {
					a.dlg.cur = i
				}
				a.fillDlgList()
				a.showDialog()
			}
		}
	case idDlgNew:
		a.newDialog()
	case idDlgDel:
		a.deleteDialog()
	case idDlgType:
		if code == CBN_SELCHANGE && !a.dlg.loading {
			a.dlg.changed = true
		}
	case idDlgParent:
		if code == CBN_SELCHANGE && !a.dlg.loading {
			a.dlg.changed = true
		}
	case idDlgNeed, idDlgGold:
		if code == EN_CHANGE && !a.dlg.loading {
			a.dlg.changed = true
		}
	case idDlgDisable:
		a.toggleDisabled()
	case idDlgPerm, idDlgClear:
		if !a.dlg.loading {
			a.dlg.changed = true
		}
	case idDlgText:
		if code == EN_CHANGE && !a.dlg.loading {
			a.dlg.changed = true
		}
	case idDlgApply:
		a.onApplyDialog()
	case idDlgQuest:
		if code == CBN_SELCHANGE && !a.dlg.loading {
			a.dlg.changed = true
			a.updateDlgButtons()
		}
	case idDlgPhase:
		if code == CBN_SELCHANGE && !a.dlg.loading {
			a.dlg.changed = true
		}
	case idAcList:
		if code == LBN_SELCHANGE || code == LBN_DBLCLK {
			a.acAccept(int(sendMessage(a.ac.list, LB_GETCURSEL, 0, 0)))
		}
	case idTabQst:
		a.setTab(3)
	case idQstList:
		if code == LBN_SELCHANGE {
			i := comboSelList(a.qst.list)
			if i != a.qst.cur {
				if a.commitQuest() {
					a.qst.cur = i
				}
				a.fillQstList()
				a.showQuest()
			}
		}
	case idQstNew:
		a.newQuest()
	case idQstDel:
		a.deleteQuest()
	case idQstPol, idQstEnc, idQstAttr, idQstSpell:
		if code == CBN_SELCHANGE && !a.qst.loading {
			a.qst.changed = true
		}
	case idQstTitle, idQstGiver, idQstTarget, idQstAmount, idQstXP, idQstGold, idQstPolAmt, idQstEncAmt, idQstAttrAmt, idQstPrereq, idQstTime:
		if code == EN_CHANGE && !a.qst.loading {
			a.qst.changed = true
		}
	case idQstType:
		if code == CBN_SELCHANGE && !a.qst.loading {
			a.qst.changed = true
			a.updateQstButtons()
			a.updateQstHelp()
		}
	case idQstPoint:
		a.onQuestPointButton()
	case idQstZone:
		a.nearbyZones()
	case idQstApply:
		a.onApplyQuest()
	case idQstGen:
		a.generateQuestDialogs()
	case idNpcSearch:
		if code == EN_CHANGE {
			a.searchNPCBases()
		}
	case idNpcList:
		if code == LBN_SELCHANGE || code == LBN_DBLCLK {
			a.selectNPCBase()
		}
	case idNpcHead:
		if code == CBN_SELCHANGE {
			a.fillHairBeard()
			a.npcPreview()
		}
	case idNpcBody, idNpcHair, idNpcBeard:
		if code == CBN_SELCHANGE {
			a.npcPreview()
		}
	case idNpcCreate:
		a.createNPC()
	case idNpcEquip:
		a.applyEquip()
	case idNpcRout:
		if code == CBN_SELCHANGE {
			a.onRoutineSelect()
		}
	case idNpcPickWork:
		a.startRoutinePick(1)
	case idNpcPickRelax:
		a.startRoutinePick(2)
	case idNpcPickSleep:
		a.startRoutinePick(3)
	case idLang:
		if code == CBN_SELCHANGE {
			i := int(sendMessage(a.hLang, CB_GETCURSEL, 0, 0))
			if i >= 0 {
				a.setLanguage(i)
			}
		}
	case idFolder:
		d := browseFolder(a.hwnd, T("Gothic-3-Ordner auswählen"))
		if d != "" {
			if !validGothicDir(d) {
				messageBox(a.hwnd, T("In diesem Ordner wurde kein Data\\Projects_compiled.pak gefunden."), T(appTitleDE), MB_ICONERROR)
				return
			}
			cfg := loadConfig()
			cfg["GothicDir"] = d
			saveConfig(cfg)
			messageBox(a.hwnd, T("Der neue Ordner wird beim nächsten Start verwendet."), T(appTitleDE), MB_ICONINFORMATION)
		}
	}
	if id == idModeWorld || id == idModeTpl {
		pSetFocus.Call(a.hSearch)
	}
	if id >= idSearch && id <= idSave && id != idSearch && id != idResults && (id < idEdX || id > idEdRot) {
		pSetFocus.Call(a.glHwnd)
	}
}

// ---------- Fenster ----------

func mainWndProc(hwnd uintptr, msg uint32, w, l uintptr) uintptr {
	a := theApp
	switch msg {
	case WM_SIZE:
		if a != nil && a.hwnd != 0 {
			a.layout()
		}
		return 0
	case WM_GETMINMAXINFO:
		mmi := (*MINMAXINFO)(unsafe.Pointer(l))
		mmi.PtMinTrackSize = POINT{800, 500}
		return 0
	case WM_SETCURSOR:
		if a != nil && loword(l) == 1 && (a.splitDrag || a.inSplitter()) {
			c, _, _ := pLoadCursorW.Call(0, IDC_SIZEWE)
			pSetCursor.Call(c)
			return 1
		}
	case WM_LBUTTONDOWN:
		if a != nil && a.inSplitter() {
			a.splitDrag = true
			pSetCapture.Call(hwnd)
			return 0
		}
	case WM_MOUSEMOVE:
		if a != nil && a.splitDrag {
			x := int(int16(loword(l)))
			var rc RECT
			pGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
			a.panelW[a.tab] = a.clampPanel(x+a.scale(4), int(rc.Right))
			a.layout()
			return 0
		}
	case WM_LBUTTONUP:
		if a != nil && a.splitDrag {
			a.splitDrag = false
			pReleaseCapture.Call()
			cfg := loadConfig()
			cfg[fmt.Sprintf("PanelWidth%d", a.tab)] = fmt.Sprintf("%d", a.panelW[a.tab])
			saveConfig(cfg)
			return 0
		}
	case WM_COMMAND:
		if a != nil && l != 0 && l == a.ac.list {
			if c := hiword(w); c == LBN_SELCHANGE || c == LBN_DBLCLK {
				a.acAccept(int(sendMessage(a.ac.list, LB_GETCURSEL, 0, 0)))
			}
			return 0
		}
		if a != nil && l != 0 {
			a.onCommand(loword(w), hiword(w))
		}
		return 0
	case 0x0112: // WM_SYSCOMMAND: Alt allein soll kein Menue oeffnen
		if w&0xFFF0 == 0xF100 && l == 0 {
			return 0
		}
	case WM_CLOSE:
		if a.ed.dirty {
			r := messageBoxRet(a.hwnd, T("Es gibt ungespeicherte Änderungen.\n\nVor dem Beenden als Mod speichern?"), T(appTitleDE), MB_YESNOCANCEL|MB_ICONWARNING)
			if r == IDCANCEL {
				return 0
			}
			if r == IDYES {
				a.save()
				if a.ed.dirty {
					return 0
				}
			}
		}
		cfg := loadConfig()
		cfg["ViewDist"] = fmt.Sprintf("%.0f", a.viewDist)
		saveConfig(cfg)
		a.shutdown()
		return 0
	case WM_DESTROY:
		if hwnd == a.hwnd {
			pPostQuitMessage.Call(0)
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), w, l)
	return r
}

func glWndProc(hwnd uintptr, msg uint32, w, l uintptr) uintptr {
	a := theApp
	switch msg {
	case WM_ERASEBKGND:
		return 1
	case WM_LBUTTONDOWN, WM_RBUTTONDOWN:
		pSetFocus.Call(hwnd)
		if msg == WM_LBUTTONDOWN && keyDown(VK_MENU) && a.selected >= 0 && a.world != nil {
			if a.inGroup() {
				a.moveGroup = a.groupMembers()
				a.moveGroupOld = groupState(a.moveGroup)
			} else {
				a.moveGroup = nil
			}
			if old, ok := a.beginMove(); ok {
				a.moving = true
				a.moveOld = old
				in := &a.world.Inst[a.selected]
				a.moveY = in.Ent.World[13]
				p := POINT{int32(loword(l)), int32(hiword(l))}
				if hit, ok := a.planeHit(p, a.moveY); ok {
					a.moveOffset = scene.Vec3{in.Ent.World[12], 0, in.Ent.World[14]}.Sub(scene.Vec3{hit[0], 0, hit[2]})
				}
				pSetCapture.Call(hwnd)
			}
			return 0
		}
		if msg == WM_LBUTTONDOWN && keyDown(VK_CONTROL) && a.world != nil {
			// Strg: Klick waehlt dazu, Ziehen zieht einen Auswahlrahmen auf
			p := POINT{int32(loword(l)), int32(hiword(l))}
			a.boxing = true
			a.boxRect = [4]int32{p.X, p.Y, p.X, p.Y}
			pSetCapture.Call(hwnd)
			return 0
		}
		pSetCapture.Call(hwnd)
		a.dragging = true
		a.dragMoved = false
		a.dragButton = 1
		if msg == WM_RBUTTONDOWN {
			a.dragButton = 2
		}
		pGetCursorPos.Call(uintptr(unsafe.Pointer(&a.dragStart)))
		a.clickPos = POINT{int32(loword(l)), int32(hiword(l))}
		return 0
	case WM_MOUSEMOVE:
		if a.boxing {
			a.boxRect[2], a.boxRect[3] = int32(loword(l)), int32(hiword(l))
			return 0
		}
		if a.moving {
			p := POINT{int32(loword(l)), int32(hiword(l))}
			if hit, ok := a.planeHit(p, a.moveY); ok && a.selected >= 0 {
				w := a.world.Inst[a.selected].Ent.World
				if a.moveGroup != nil {
					d := scene.Vec3{hit[0] + a.moveOffset[0] - w[12], 0, hit[2] + a.moveOffset[2] - w[14]}
					a.translateGroup(a.moveGroup, d)
					a.select_(a.selected, false)
				} else {
					w[12], w[14] = hit[0]+a.moveOffset[0], hit[2]+a.moveOffset[2]
					a.setSelectedWorld(w, false)
				}
			}
			return 0
		}
		if a.dragging {
			var p POINT
			pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
			dx, dy := p.X-a.dragStart.X, p.Y-a.dragStart.Y
			if !a.dragMoved && dx*dx+dy*dy > 9 {
				a.dragMoved = true
				pShowCursor.Call(0)
			}
			if a.dragMoved && (dx != 0 || dy != 0) {
				a.cam.Yaw += float32(dx) * 0.0035
				a.cam.Pitch -= float32(dy) * 0.0035
				if a.cam.Pitch > 1.55 {
					a.cam.Pitch = 1.55
				}
				if a.cam.Pitch < -1.55 {
					a.cam.Pitch = -1.55
				}
				pSetCursorPos.Call(uintptr(a.dragStart.X), uintptr(a.dragStart.Y))
			}
		}
		return 0
	case WM_LBUTTONUP, WM_RBUTTONUP:
		if a.boxing {
			a.boxing = false
			pReleaseCapture.Call()
			r := a.boxRect
			dx, dy := r[2]-r[0], r[3]-r[1]
			if dx*dx+dy*dy > 25 {
				a.boxReq = &r
			} else {
				p := POINT{r[0], r[1]}
				a.pickAt = &p
				a.pickCtrl = true
			}
			return 0
		}
		if a.moving {
			a.moving = false
			pReleaseCapture.Call()
			a.finishMove()
			return 0
		}
		if a.dragging {
			a.dragging = false
			pReleaseCapture.Call()
			if a.dragMoved {
				pShowCursor.Call(1)
			} else if msg == WM_LBUTTONUP {
				p := a.clickPos
				a.pickAt = &p
				a.pickCtrl = false
			}
		}
		return 0
	case WM_CAPTURECHANGED:
		a.boxing = false
		if a.moving {
			a.moving = false
			a.finishMove()
		}
		if a.dragging {
			a.dragging = false
			if a.dragMoved {
				pShowCursor.Call(1)
			}
		}
		return 0
	case WM_MOUSEWHEEL:
		if keyDown(VK_MENU) && a.selected >= 0 {
			step := float32(10)
			if keyDown(VK_SHIFT) {
				step = 100
			}
			if hiword(w) < 0 {
				step = -step
			}
			a.translateSelected(scene.Vec3{0, step, 0}, true)
			return 0
		}
		if hiword(w) > 0 {
			a.speed *= 1.25
		} else {
			a.speed /= 1.25
		}
		if a.speed < 100 {
			a.speed = 100
		}
		if a.speed > 40000 {
			a.speed = 40000
		}
		return 0
	case WM_KEYDOWN:
		switch int(w) {
		case 'F':
			if keyDown(VK_SHIFT) {
				a.dropSelected()
			} else if a.selected >= 0 {
				a.select_(a.selected, true)
			}
		case VK_ESCAPE:
			if a.npc.pick > 0 {
				a.npc.pick = 0
				a.flash(T("Abgebrochen."))
				return 0
			}
			a.select_(-1, false)
			a.prev = nil
		case VK_F1:
			a.showHelp()
		case VK_PRIOR:
			a.setViewDist(a.viewDist * 1.25)
		case VK_NEXT:
			a.setViewDist(a.viewDist / 1.25)
		case VK_DELETE:
			a.deleteSelected()
		case 'R':
			deg := 15.0
			if keyDown(VK_CONTROL) {
				deg = 1
			}
			if keyDown(VK_SHIFT) {
				deg = -deg
			}
			a.rotateSelected(deg)
		case 'C':
			if keyDown(VK_CONTROL) {
				a.copySelected()
			}
		case 'V':
			if keyDown(VK_CONTROL) {
				a.pasteClip()
			}
		case 'D':
			if keyDown(VK_CONTROL) {
				a.duplicateSelected()
			}
		case 'Z':
			if keyDown(VK_CONTROL) && keyDown(VK_SHIFT) {
				a.redoLast()
			} else if keyDown(VK_CONTROL) {
				a.undoLast()
			}
		case 'Y':
			if keyDown(VK_CONTROL) {
				a.redoLast()
			}
		case 'S':
			if keyDown(VK_CONTROL) {
				a.save()
			}
		}
		return 0
	case WM_SIZE:
		a.width, a.height = loword(l)&0xffff, hiword(l)&0xffff
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), w, l)
	return r
}

func registerClass(name string, proc uintptr, style uint32, bg uintptr) error {
	hinst, _, _ := pGetModuleHandleW.Call(0)
	cur, _, _ := pLoadCursorW.Call(0, IDC_ARROW)
	// eigenes Programmsymbol (Ressource "APP"), sonst das Windows-Standardsymbol
	icon, _, _ := pLoadIconW.Call(hinst, uintptr(unsafe.Pointer(u16("APP"))))
	if icon == 0 {
		icon, _, _ = pLoadIconW.Call(0, IDI_APPLICATION)
	}
	wc := WNDCLASSEXW{CbSize: uint32(unsafe.Sizeof(WNDCLASSEXW{})), Style: style, LpfnWndProc: proc,
		HInstance: hinst, HCursor: cur, HIcon: icon, HIconSm: icon, HbrBackground: bg, LpszClassName: u16(name)}
	if r, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return fmt.Errorf("RegisterClass %s: %v", name, err)
	}
	return nil
}

func (a *app) createWindows() error {
	// COLOR_BTNFACE+1 = 16 als Hintergrund
	if err := registerClass("AdanosEditMain", syscall.NewCallback(mainWndProc), CS_HREDRAW|CS_VREDRAW, 16); err != nil {
		return err
	}
	if err := registerClass("AdanosEditGL", syscall.NewCallback(glWndProc), CS_OWNDC|CS_DBLCLKS, 0); err != nil {
		return err
	}
	a.hwnd = createWindow(0, "AdanosEditMain", T(appTitleDE)+" "+version, WS_OVERLAPPEDWINDOW|WS_CLIPCHILDREN,
		CW_USEDEFAULT, CW_USEDEFAULT, 1400, 900, 0, 0)
	if a.hwnd == 0 {
		return fmt.Errorf("Hauptfenster konnte nicht erstellt werden")
	}
	if pGetDpiForWindow.Find() == nil {
		if dpi, _, _ := pGetDpiForWindow.Call(a.hwnd); dpi > 0 {
			a.dpi = float32(dpi) / 96
		}
	}
	a.font, _, _ = pCreateFontW.Call(uintptr(-a.scale(15)), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(u16("Segoe UI"))))
	child := uint32(WS_CHILD | WS_VISIBLE)
	mk := func(ex uint32, class, text string, style uint32, id int) uintptr {
		h := createWindow(ex, class, text, child|style, 0, 0, 10, 10, a.hwnd, uintptr(id))
		sendMessage(h, WM_SETFONT, a.font, 1)
		return h
	}
	a.glHwnd = createWindow(0, "AdanosEditGL", "", WS_CHILD|WS_VISIBLE|WS_CLIPSIBLINGS|WS_CLIPCHILDREN, 0, 0, 10, 10, a.hwnd, 1)
	a.createNPCControls(mk)
	a.createDlgControls(mk)
	a.createQstControls(mk)
	a.createAutoComplete(mk)
	a.createPlaceControls(mk)
	a.ac.fields = []uintptr{a.npc.items, a.qst.target, a.qst.place, a.qst.prereq, a.dlg.cond, a.dlg.condNot, a.dlg.need}
	a.ac.source = map[uintptr]func() []acItem{a.dlg.cond: a.eventItems, a.dlg.condNot: a.eventItems, a.qst.place: a.zoneItems, a.qst.prereq: a.questItems}
	a.ac.suffix = map[uintptr]string{a.dlg.cond: "", a.dlg.condNot: "", a.qst.place: "", a.qst.prereq: ""}
	a.hModeWorld = mk(0, "BUTTON", T("In der Welt suchen"), BS_AUTORADIOBUTTON|WS_GROUP|WS_TABSTOP, idModeWorld)
	a.hModeTpl = mk(0, "BUTTON", T("Neues Objekt aus Vorlage anlegen"), BS_AUTORADIOBUTTON, idModeTpl)
	setChecked(a.hModeWorld, true)
	a.hSearchLbl = mk(0, "STATIC", T("Objekt suchen (Name oder Modell):"), SS_LEFT, 0)
	a.hSearch = mk(WS_EX_CLIENTEDGE, "EDIT", "", ES_AUTOHSCROLL|WS_TABSTOP, idSearch)
	a.hResults = mk(WS_EX_CLIENTEDGE, "LISTBOX", "", WS_VSCROLL|LBS_NOTIFY|LBS_NOINTEGRALHEIGHT|WS_TABSTOP, idResults)
	a.hPlace = mk(0, "BUTTON", T("Neu platzieren (in der Bildmitte)"), BS_PUSHBUTTON, idPlace)
	pEnableWindow.Call(a.hPlace, 0)
	a.hChk = map[int]uintptr{}
	for id := idChkNPC; id <= idChkWire; id++ {
		a.hChk[id] = mk(0, "BUTTON", T(chkLabels[id]), BS_AUTOCHECKBOX|WS_TABSTOP, id)
	}
	setChecked(a.hChk[idChkNPC], true)
	setChecked(a.hChk[idChkItems], true)
	setChecked(a.hChk[idChkTrees], true)
	setChecked(a.hChk[idChkTextures], true)
	setChecked(a.hChk[idChkWater], true)
	setChecked(a.hChk[idChkBarrier], true)
	a.showWater, a.showBarrier = true, true
	a.hDistLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	a.hDistMinus = mk(0, "BUTTON", "–", BS_PUSHBUTTON, idDistMinus)
	a.hDistPlus = mk(0, "BUTTON", "+", BS_PUSHBUTTON, idDistPlus)
	a.hInfoLbl = mk(0, "STATIC", T("Ausgewähltes Objekt:"), SS_LEFT, 0)
	a.hInfo = mk(WS_EX_CLIENTEDGE, "EDIT", "", ES_MULTILINE|ES_READONLY|ES_AUTOVSCROLL|WS_VSCROLL, idInfo)
	a.hHelp = mk(0, "BUTTON", T("Hilfe (F1)"), BS_PUSHBUTTON, idHelp)
	a.hFolder = mk(0, "BUTTON", T("Gothic-Ordner …"), BS_PUSHBUTTON, idFolder)
	a.hEditLbl = mk(0, "STATIC", T("Bearbeiten (Position in cm, Drehung in Grad):"), SS_LEFT, 0)
	a.hLblX = mk(0, "STATIC", "X", SS_LEFT, 0)
	a.hLblY = mk(0, "STATIC", "Y", SS_LEFT, 0)
	a.hLblZ = mk(0, "STATIC", "Z", SS_LEFT, 0)
	a.hLblRot = mk(0, "STATIC", T("Drehung"), SS_LEFT, 0)
	a.hEdX = mk(WS_EX_CLIENTEDGE, "EDIT", "", ES_AUTOHSCROLL|WS_TABSTOP, idEdX)
	a.hEdY = mk(WS_EX_CLIENTEDGE, "EDIT", "", ES_AUTOHSCROLL|WS_TABSTOP, idEdY)
	a.hEdZ = mk(WS_EX_CLIENTEDGE, "EDIT", "", ES_AUTOHSCROLL|WS_TABSTOP, idEdZ)
	a.hEdRot = mk(WS_EX_CLIENTEDGE, "EDIT", "", ES_AUTOHSCROLL|WS_TABSTOP, idEdRot)
	a.hApply = mk(0, "BUTTON", T("Übernehmen"), BS_PUSHBUTTON, idApply)
	a.hCopy = mk(0, "BUTTON", T("Kopieren"), BS_PUSHBUTTON, idCopy)
	a.hDelete = mk(0, "BUTTON", T("Löschen"), BS_PUSHBUTTON, idDelete)
	a.hUndo = mk(0, "BUTTON", T("Rückgängig"), BS_PUSHBUTTON, idUndo)
	a.hClearNPC = mk(0, "BUTTON", T("NPCs entfernen …"), BS_PUSHBUTTON, idClearNPC)
	a.hChest = mk(0, "BUTTON", T("Truhe / Schloss …"), BS_PUSHBUTTON, idChest)
	a.hModLbl = mk(0, "STATIC", T("Mod:"), SS_LEFT, 0)
	a.hMod = mk(0, "COMBOBOX", "", CBS_DROPDOWNLIST|WS_VSCROLL|WS_TABSTOP, idMod)
	a.hSave = mk(0, "BUTTON", T("Änderungen als Mod speichern"), BS_PUSHBUTTON, idSave)
	a.hCheck = mk(0, "BUTTON", T("Mod prüfen …"), BS_PUSHBUTTON, idModCheck)
	a.hStatus = mk(0, "STATIC", T("Starte …"), SS_LEFTNOWORDWRAP, 0)
	a.hLangLbl = mk(0, "STATIC", T("Sprache:"), SS_LEFT, 0)
	a.hLang = mk(0, "COMBOBOX", "", CBS_DROPDOWNLIST|WS_VSCROLL|WS_TABSTOP, idLang)
	for _, n := range lang.Names {
		sendMessage(a.hLang, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(n))))
	}
	sendMessage(a.hLang, CB_SETCURSEL, uintptr(lang.Current), 0)
	a.hFlag = mk(0, "STATIC", "", ssBitmap, 0)
	a.updateFlag()
	a.setTab(0)
	a.setViewDist(a.viewDist)
	pShowWindow.Call(a.hwnd, SW_SHOWMAXIMIZED)
	pUpdateWindow.Call(a.hwnd)
	a.layout()
	pSetFocus.Call(a.glHwnd)
	return nil
}

func (a *app) layout() {
	var rc RECT
	pGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	W, H := int(rc.Right), int(rc.Bottom)
	pw := a.panelWidth(W)
	m := a.scale(8)
	sh := a.scale(24)
	moveWindow(a.glHwnd, pw, 0, W-pw, H-sh)
	moveWindow(a.hStatus, m, H-sh+a.scale(4), W-2*m, sh-a.scale(4))
	x, w := m, pw-2*m
	y := m
	lh := a.scale(20)
	ch := a.scale(22)
	// Reiter
	tw := (w - a.scale(12)) / 4
	moveWindow(a.hTabWorld, x, y, tw, a.scale(28))
	moveWindow(a.hTabNPC, x+tw+a.scale(4), y, tw, a.scale(28))
	moveWindow(a.hTabDlg, x+2*tw+a.scale(8), y, tw, a.scale(28))
	moveWindow(a.hTabQst, x+3*tw+a.scale(12), y, w-3*tw-a.scale(12), a.scale(28))
	y += a.scale(36)
	if a.tab >= 1 {
		bh := a.scale(28)
		bottom := H - sh - m
		yb := bottom - bh
		moveWindow(a.hHelp, x, yb, w/2-a.scale(3), bh)
		moveWindow(a.hFolder, x+w/2+a.scale(3), yb, w/2-a.scale(3), bh)
		yb -= bh + m
		a.layoutLang(x, yb, w, lh)
		yb -= bh + a.scale(8)
		a.layoutSave(x, yb, w, bh)
		yb -= bh + a.scale(4)
		moveWindow(a.hModLbl, x, yb+a.scale(5), a.scale(40), lh)
		moveWindow(a.hMod, x+a.scale(42), yb, w-a.scale(42), a.scale(240))
		yb -= bh + a.scale(4)
		a.layoutPlaces(x, yb, w)
		switch a.tab {
		case 1:
			a.layoutNPC(x, y, w, yb-a.scale(10))
		case 2:
			a.layoutDlg(x, y, w, yb-a.scale(10))
		default:
			a.layoutQst(x, y, w, yb-a.scale(10))
		}
		return
	}
	moveWindow(a.hModeWorld, x, y, w, ch)
	y += ch
	moveWindow(a.hModeTpl, x, y, w, ch)
	y += ch + a.scale(4)
	moveWindow(a.hSearchLbl, x, y, w, lh)
	y += lh
	moveWindow(a.hSearch, x, y, w, ch+a.scale(2))
	y += ch + a.scale(6)
	listH := (H - sh) * 18 / 100
	moveWindow(a.hResults, x, y, w, listH)
	y += listH + a.scale(4)
	moveWindow(a.hPlace, x, y, w, ch+a.scale(4))
	y += ch + a.scale(10)
	for id := idChkNPC; id <= idChkWire; id++ {
		moveWindow(a.hChk[id], x, y, w, ch)
		y += ch
	}
	y += a.scale(6)
	bw := a.scale(30)
	moveWindow(a.hDistLbl, x, y+a.scale(4), w-2*bw-a.scale(8), lh)
	moveWindow(a.hDistMinus, x+w-2*bw-a.scale(4), y, bw, ch+a.scale(4))
	moveWindow(a.hDistPlus, x+w-bw, y, bw, ch+a.scale(4))
	y += ch + a.scale(14)
	// Bearbeiten
	moveWindow(a.hEditLbl, x, y, w, lh)
	y += lh
	ew := (w - a.scale(3*16)) / 3
	for i, pair := range [][2]uintptr{{a.hLblX, a.hEdX}, {a.hLblY, a.hEdY}, {a.hLblZ, a.hEdZ}} {
		xx := x + i*(ew+a.scale(16))
		moveWindow(pair[0], xx, y+a.scale(3), a.scale(14), lh)
		moveWindow(pair[1], xx+a.scale(14), y, ew, ch)
	}
	y += ch + a.scale(4)
	moveWindow(a.hLblRot, x, y+a.scale(3), a.scale(60), lh)
	moveWindow(a.hEdRot, x+a.scale(62), y, a.scale(70), ch)
	moveWindow(a.hApply, x+a.scale(140), y-a.scale(1), w-a.scale(140), ch+a.scale(2))
	y += ch + a.scale(6)
	bw3 := (w - a.scale(8)) / 3
	bh := a.scale(28)
	moveWindow(a.hCopy, x, y, bw3, bh)
	moveWindow(a.hDelete, x+bw3+a.scale(4), y, bw3, bh)
	moveWindow(a.hUndo, x+2*(bw3+a.scale(4)), y, bw3, bh)
	y += bh + a.scale(4)
	hw2 := (w - a.scale(4)) / 2
	moveWindow(a.hClearNPC, x, y, hw2, bh)
	moveWindow(a.hChest, x+hw2+a.scale(4), y, w-hw2-a.scale(4), bh)
	y += bh + a.scale(4)
	a.layoutPlaces(x, y, w)
	y += bh + a.scale(4)
	moveWindow(a.hModLbl, x, y+a.scale(5), a.scale(40), lh)
	moveWindow(a.hMod, x+a.scale(42), y, w-a.scale(42), a.scale(240))
	y += bh + a.scale(4)
	a.layoutSave(x, y, w, bh)
	y += bh + a.scale(12)
	moveWindow(a.hInfoLbl, x, y, w, lh)
	y += lh
	infoH := H - sh - y - 2*bh - 3*m
	if infoH < a.scale(40) {
		infoH = a.scale(40)
	}
	moveWindow(a.hInfo, x, y, w, infoH)
	y += infoH + m
	a.layoutLang(x, y, w, lh)
	y += bh + m
	moveWindow(a.hHelp, x, y, w/2-a.scale(3), bh)
	moveWindow(a.hFolder, x+w/2+a.scale(3), y, w/2-a.scale(3), bh)
}

func (a *app) initGL() error {
	dc, _, _ := pGetDC.Call(a.glHwnd)
	a.hdc = dc
	pfd := PIXELFORMATDESCRIPTOR{NSize: uint16(unsafe.Sizeof(PIXELFORMATDESCRIPTOR{})), NVersion: 1,
		DwFlags: PFD_DRAW_TO_WINDOW | PFD_SUPPORT_OPENGL | PFD_DOUBLEBUFFER, IPixelType: PFD_TYPE_RGBA,
		CColorBits: 32, CAlphaBits: 8, CDepthBits: 24, CStencilBits: 8, ILayerType: PFD_MAIN_PLANE}
	pf, _, _ := pChoosePixelFormat.Call(dc, uintptr(unsafe.Pointer(&pfd)))
	if pf == 0 {
		return fmt.Errorf("kein passendes Pixelformat")
	}
	if r, _, _ := pSetPixelFormat.Call(dc, pf, uintptr(unsafe.Pointer(&pfd))); r == 0 {
		return fmt.Errorf("SetPixelFormat fehlgeschlagen")
	}
	rc, _, _ := pwglCreateContext.Call(dc)
	if rc == 0 {
		return fmt.Errorf("wglCreateContext fehlgeschlagen")
	}
	if r, _, _ := pwglMakeCurrent.Call(dc, rc); r == 0 {
		return fmt.Errorf("wglMakeCurrent fehlgeschlagen")
	}
	a.hglrc = rc
	log.Printf("OpenGL: %s | %s | %s", glGetString(GL_VENDOR), glGetString(GL_RENDERER), glGetString(GL_VERSION))
	if err := loadExtensions(); err != nil {
		return err
	}
	if !hasCompressed {
		log.Println("Warnung: keine DXT-Texturunterstuetzung")
	}
	wglSwapInterval(1)
	glHint(GL_PERSPECTIVE_CORRECTION_HINT, GL_NICEST)
	var rcl RECT
	pGetClientRect.Call(a.glHwnd, uintptr(unsafe.Pointer(&rcl)))
	a.width, a.height = int(rcl.Right), int(rcl.Bottom)
	return nil
}

func enableVisualStyles() {
	dir := os.TempDir()
	p := filepath.Join(dir, "g3viewer_styles.manifest")
	if err := os.WriteFile(p, []byte(manifestXML), 0644); err != nil {
		return
	}
	ctx := ACTCTXW{CbSize: uint32(unsafe.Sizeof(ACTCTXW{})), LpSource: u16(p)}
	h, _, _ := pCreateActCtxW.Call(uintptr(unsafe.Pointer(&ctx)))
	if h == 0 || h == ^uintptr(0) {
		return
	}
	var cookie uintptr
	pActivateActCtx.Call(h, uintptr(unsafe.Pointer(&cookie)))
}

// ---------- Konfiguration ----------

func exeDir() string {
	p, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(p)
}

func configDirs() []string {
	dirs := []string{exeDir()}
	if ad := os.Getenv("APPDATA"); ad != "" {
		dirs = append(dirs, filepath.Join(ad, "AdanosEdit"), filepath.Join(ad, "G3Viewer"))
	}
	return dirs
}

func loadConfig() map[string]string {
	cfg := map[string]string{}
	for _, f := range configFiles() {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if i := strings.Index(line, "="); i > 0 {
				cfg[strings.TrimSpace(line[:i])] = strings.TrimSpace(line[i+1:])
			}
		}
		break
	}
	return cfg
}

func saveConfig(cfg map[string]string) {
	var keys []string
	for k := range cfg {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("; Einstellungen von AdanosEdit\r\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s\r\n", k, cfg[k])
	}
	for _, d := range configDirs() {
		os.MkdirAll(d, 0755)
		if os.WriteFile(filepath.Join(d, configName), []byte(b.String()), 0644) == nil {
			return
		}
	}
}

func setupLog() {
	for _, d := range configDirs() {
		os.MkdirAll(d, 0755)
		f, err := os.Create(filepath.Join(d, "AdanosEdit.log"))
		if err == nil {
			log.SetOutput(f)
			log.SetFlags(log.Ltime)
			log.Printf("AdanosEdit %s gestartet", version)
			return
		}
	}
}

func validGothicDir(d string) bool {
	_, err := os.Stat(filepath.Join(d, "Data", "Projects_compiled.pak"))
	return err == nil
}

func findGothicDir(fromCfg string) string {
	cands := []string{fromCfg, filepath.Dir(exeDir()), exeDir()}
	for _, pf := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles"), `C:\Program Files (x86)`, `C:\Program Files`} {
		if pf == "" {
			continue
		}
		cands = append(cands, filepath.Join(pf, "Steam", "steamapps", "common", "Gothic 3"),
			filepath.Join(pf, "GOG Galaxy", "Games", "Gothic 3"), filepath.Join(pf, "JoWooD", "Gothic 3"))
	}
	cands = append(cands, `C:\GOG Games\Gothic 3`, `D:\SteamLibrary\steamapps\common\Gothic 3`, `E:\SteamLibrary\steamapps\common\Gothic 3`)
	for _, c := range cands {
		if c != "" && validGothicDir(c) {
			return c
		}
	}
	return ""
}

// panelWidth liefert die Breite der linken Leiste fuer den aktuellen Reiter.
// Sie laesst sich mit der Maus am rechten Rand der Leiste verstellen.
func (a *app) panelWidth(W int) int {
	if a.panelW[a.tab] == 0 {
		if v, err := strconv.Atoi(loadConfig()[fmt.Sprintf("PanelWidth%d", a.tab)]); err == nil && v > 0 {
			a.panelW[a.tab] = v
		}
	}
	pw := a.panelW[a.tab]
	if pw == 0 {
		switch a.tab {
		case 0:
			pw = a.scale(300)
		case 1:
			pw = a.scale(360)
		default:
			pw = a.scale(440)
		}
	}
	return a.clampPanel(pw, W)
}

func (a *app) clampPanel(pw, W int) int {
	if mx := W - a.scale(320); pw > mx {
		pw = mx
	}
	if mn := a.scale(240); pw < mn {
		pw = mn
	}
	return pw
}

// inSplitter: Mauszeiger steht auf dem Rand zwischen Leiste und 3D-Ansicht.
func (a *app) inSplitter() bool {
	var pt POINT
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pScreenToClient.Call(a.hwnd, uintptr(unsafe.Pointer(&pt)))
	var rc RECT
	pGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	pw := a.panelWidth(int(rc.Right))
	x := int(pt.X)
	return x >= pw-a.scale(8) && x < pw && int(pt.Y) >= 0 && int(pt.Y) < int(rc.Bottom)-a.scale(24)
}

// Name der Einstellungsdatei; die alte G3Viewer.ini wird beim ersten Start uebernommen.
const configName = "AdanosEdit.ini"

func configFiles() []string {
	var out []string
	for _, d := range configDirs() {
		out = append(out, filepath.Join(d, configName))
	}
	for _, d := range configDirs() {
		out = append(out, filepath.Join(d, "G3Viewer.ini"))
	}
	return out
}

// instanceMutex verhindert einen zweiten Start (wird beim Neustart freigegeben).
var instanceMutex uintptr
