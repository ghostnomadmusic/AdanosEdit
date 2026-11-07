//go:build windows

package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"g3viewer/g3"
	"g3viewer/scene"
)

// editor verwaltet geaenderte Weltdateien und die Rueckgaengig-Liste.
type editor struct {
	files map[string]*g3.WorldFile // Pfad im Archiv -> Datei
	inst  map[string]map[int]int32 // Pfad -> Entity-Index -> Instanz
	undo  []func()
	// Wiederherstellen (redo.go)
	undoKeys [][]entKey      // je Rueckgaengig-Schritt die veraenderten Objekte
	pending  map[entKey]bool // seit dem letzten Schritt veraenderte Objekte
	redo     []redoEntry
	redoPush bool
	dirty    bool
	// weitere Archive der Mod (z. B. "Strings" -> "stringtable.ini" -> Inhalt)
	extra map[string]map[string][]byte
	// neue Texte (Schluessel -> Texte je Sprache) fuer Data\Strings\stringtable.ini
	strs map[string][]string
}

// setString merkt sich einen Text fuer die stringtable.ini.
func (ed *editor) setString(key string, texts []string) {
	if ed.strs == nil {
		ed.strs = map[string][]string{}
	}
	ed.strs[key] = texts
	ed.dirty = true
}

// setExtra merkt sich eine Datei fuer ein weiteres Archiv der Mod.
func (ed *editor) setExtra(cat, path string, d []byte) {
	if ed.extra == nil {
		ed.extra = map[string]map[string][]byte{}
	}
	if ed.extra[cat] == nil {
		ed.extra[cat] = map[string][]byte{}
	}
	ed.extra[cat][path] = d
	ed.dirty = true
}

func newEditor() *editor {
	return &editor{files: map[string]*g3.WorldFile{}, inst: map[string]map[int]int32{}}
}

// file oeffnet (einmalig) die Datei einer Instanz zum Bearbeiten.
func (a *app) editFile(in *scene.Instance) (*g3.WorldFile, error) {
	ed := a.ed
	path := in.Ent.Source
	if wf := ed.files[path]; wf != nil {
		return wf, nil
	}
	e := a.vfs.Find("Projects_compiled", path)
	if e == nil {
		return nil, fmt.Errorf(T("Datei %s nicht gefunden"), path)
	}
	d, err := e.Read()
	if err != nil {
		return nil, err
	}
	wf, err := g3.OpenWorldFile(d, path)
	if err != nil {
		return nil, err
	}
	m := map[int]int32{}
	for i := range a.world.Inst {
		if a.world.Inst[i].Ent.Source == path {
			m[a.world.Inst[i].Ent.Index] = int32(i)
		}
	}
	ed.files[path] = wf
	ed.inst[path] = m
	return wf, nil
}

// syncFile uebernimmt die Daten einer bearbeiteten Datei in die Darstellung.
func (a *app) syncFile(wf *g3.WorldFile) {
	m := a.ed.inst[wf.Path]
	a.noteChanged(wf, wf.Changed)
	for k := range wf.Changed {
		i, ok := m[k]
		if !ok {
			continue
		}
		a.world.Update(i, wf.Ents[k])
		a.world.Inst[i].Hidden = wf.Deleted[k]
	}
	wf.Changed = nil
	a.ed.dirty = true
	a.updateTitle()
}

func (a *app) updateTitle() {
	t := T(appTitleDE) + " " + version + "  –  " + a.modTitle()
	if a.ed.dirty {
		t += T("  –  ungespeicherte Änderungen")
	}
	setText(a.hwnd, t)
}

func (a *app) selectedEditable() (*scene.Instance, *g3.WorldFile, bool) {
	if a.world == nil || a.selected < 0 {
		return nil, nil, false
	}
	in := &a.world.Inst[a.selected]
	wf, err := a.editFile(in)
	if err != nil {
		a.flash(fmt.Sprintf(T("Dieses Objekt kann nicht bearbeitet werden: %s"), T(err.Error())))
		return nil, nil, false
	}
	return in, wf, true
}

// moveSelected setzt die Weltmatrix des ausgewaehlten Objekts.
func (a *app) setSelectedWorld(world [16]float32, record bool) {
	in, wf, ok := a.selectedEditable()
	if !ok {
		return
	}
	k := in.Ent.Index
	old := wf.Ents[k].World
	wf.SetWorld(k, world)
	a.syncFile(wf)
	if record {
		a.ed.undo = append(a.ed.undo, func() {
			wf.SetWorld(k, old)
			a.syncFile(wf)
		})
	}
	a.select_(a.selected, false)
}

// beginDrag merkt sich den Ausgangszustand fuer eine Mausverschiebung.
func (a *app) beginMove() ([16]float32, bool) {
	in, wf, ok := a.selectedEditable()
	if !ok {
		return [16]float32{}, false
	}
	return wf.Ents[in.Ent.Index].World, true
}

func (a *app) endMove(old [16]float32) {
	in, wf, ok := a.selectedEditable()
	if !ok {
		return
	}
	k := in.Ent.Index
	if wf.Ents[k].World == old {
		return
	}
	a.ed.undo = append(a.ed.undo, func() {
		wf.SetWorld(k, old)
		a.syncFile(wf)
	})
}

func (a *app) translateSelected(d scene.Vec3, record bool) {
	if a.selected < 0 {
		return
	}
	if a.inGroup() {
		a.translateGroupUndo(d)
		return
	}
	w := a.world.Inst[a.selected].Ent.World
	w[12] += d[0]
	w[13] += d[1]
	w[14] += d[2]
	a.setSelectedWorld(w, record)
}

// rotateSelected dreht um die senkrechte Achse (Grad).
func (a *app) rotateSelected(deg float64) {
	if a.selected < 0 {
		return
	}
	if a.inGroup() {
		a.rotateGroup(deg)
		return
	}
	w := a.world.Inst[a.selected].Ent.World
	r := deg * math.Pi / 180
	c, s := float32(math.Cos(r)), float32(math.Sin(r))
	rot := [16]float32{c, 0, -s, 0, 0, 1, 0, 0, s, 0, c, 0, 0, 0, 0, 1}
	pos := [3]float32{w[12], w[13], w[14]}
	w[12], w[13], w[14] = 0, 0, 0
	w = g3.MatMul(w, rot)
	w[12], w[13], w[14] = pos[0], pos[1], pos[2]
	a.setSelectedWorld(w, true)
}

func (a *app) deleteSelected() {
	if a.inGroup() {
		a.deleteGroup()
		return
	}
	in, wf, ok := a.selectedEditable()
	if !ok {
		return
	}
	k := in.Ent.Index
	if k == 0 {
		a.flash(T("Die Wurzel einer Datei kann nicht gelöscht werden."))
		return
	}
	if in.Ent.Kind == g3.KindNPC {
		r := messageBoxRet(a.hwnd, T("Dieses Objekt ist ein NPC. Skripte oder Quests können auf ihn verweisen und im Spiel Fehler auslösen.\n\nTrotzdem löschen?"), T(appTitleDE), MB_YESNO|MB_ICONWARNING)
		if r != IDYES {
			return
		}
	}
	if err := wf.Delete(k); err != nil {
		a.flash(T(err.Error()))
		return
	}
	a.syncFile(wf)
	a.ed.undo = append(a.ed.undo, func() {
		wf.Undelete(k)
		a.syncFile(wf)
	})
	a.select_(-1, false)
	a.flash(T("Objekt gelöscht (Strg+Z macht es rückgängig)."))
}

// duplicateSelected kopiert das Objekt an die Stelle in der Bildmitte.
func (a *app) duplicateSelected() {
	if a.groupOnlyOne() {
		return
	}
	in, wf, ok := a.selectedEditable()
	if !ok {
		return
	}
	ci, err := wf.Clone(in.Ent.Index)
	if err != nil {
		a.flash(T(err.Error()))
		return
	}
	ne := wf.Ents[ci]
	ii := a.world.Add(ne, in.Sector)
	if ii < 0 {
		wf.Delete(ci)
		a.flash(T("Die Kopie konnte nicht angelegt werden."))
		return
	}
	a.ed.inst[wf.Path][ci] = ii
	w := ne.World
	if p, ok := a.centerPoint(); ok {
		w[12], w[13], w[14] = p[0], p[1], p[2]
	} else {
		f := a.cam.Forward()
		w[12] += f[0] * 150
		w[14] += f[2] * 150
	}
	wf.SetWorld(ci, w)
	a.syncFile(wf)
	a.ed.undo = append(a.ed.undo, func() {
		wf.Delete(ci)
		a.syncFile(wf)
		if a.selected == ii {
			a.select_(-1, false)
		}
	})
	a.select_(ii, false)
	a.flash(T("Kopie angelegt. Mit Alt + linker Maustaste verschieben, R dreht."))
}

func (a *app) undoLast() {
	if len(a.ed.undo) == 0 {
		a.flash(T("Nichts zum Rückgängigmachen."))
		return
	}
	a.popUndo()()
	if a.selected >= 0 {
		a.select_(a.selected, false)
	}
}

// applyFields uebernimmt die Eingabefelder (Position, Drehung).
func (a *app) applyFields() {
	if a.selected < 0 {
		return
	}
	var x, y, z, rot float64
	if _, err := fmt.Sscan(strings.ReplaceAll(getText(a.hEdX), ",", "."), &x); err != nil {
		a.flash(T("Ungültige X-Koordinate"))
		return
	}
	if _, err := fmt.Sscan(strings.ReplaceAll(getText(a.hEdY), ",", "."), &y); err != nil {
		a.flash(T("Ungültige Y-Koordinate"))
		return
	}
	if _, err := fmt.Sscan(strings.ReplaceAll(getText(a.hEdZ), ",", "."), &z); err != nil {
		a.flash(T("Ungültige Z-Koordinate"))
		return
	}
	if _, err := fmt.Sscan(strings.ReplaceAll(getText(a.hEdRot), ",", "."), &rot); err != nil {
		a.flash(T("Ungültige Drehung"))
		return
	}
	w := a.world.Inst[a.selected].Ent.World
	cur := math.Atan2(float64(w[8]), float64(w[10])) * 180 / math.Pi
	if a.inGroup() {
		// Hauptobjekt an die Stelle, die anderen behalten ihren Abstand
		if d := (scene.Vec3{float32(x) - w[12], float32(y) - w[13], float32(z) - w[14]}); d != (scene.Vec3{}) {
			a.translateSelected(d, true)
		}
	} else {
		w[12], w[13], w[14] = float32(x), float32(y), float32(z)
		a.setSelectedWorld(w, true)
	}
	if d := rot - cur; math.Abs(d) > 0.01 {
		a.rotateSelected(-d)
	}
}

func (a *app) fillFields() {
	if a.selected < 0 || a.world == nil {
		for _, h := range []uintptr{a.hEdX, a.hEdY, a.hEdZ, a.hEdRot} {
			setText(h, "")
		}
		return
	}
	w := a.world.Inst[a.selected].Ent.World
	setText(a.hEdX, fmt.Sprintf("%.1f", w[12]))
	setText(a.hEdY, fmt.Sprintf("%.1f", w[13]))
	setText(a.hEdZ, fmt.Sprintf("%.1f", w[14]))
	setText(a.hEdRot, fmt.Sprintf("%.1f", math.Atan2(float64(w[8]), float64(w[10]))*180/math.Pi))
}

// modTarget bestimmt die Mod-Datei im Data-Ordner.
func (a *app) modTarget() string {
	data := filepath.Join(a.gothicDir, "Data")
	cfg := loadConfig()
	if m := cfg["ModFile"]; m != "" {
		if _, err := os.Stat(filepath.Join(data, m)); err == nil {
			return filepath.Join(data, m)
		}
	}
	if _, err := os.Stat(filepath.Join(data, "Projects_compiled.mod")); os.IsNotExist(err) {
		return filepath.Join(data, "Projects_compiled.mod")
	}
	for i := 0; i < 100; i++ {
		p := filepath.Join(data, fmt.Sprintf("Projects_compiled.m%02d", i))
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return p
		}
	}
	return filepath.Join(data, "Projects_compiled.m99")
}

// save schreibt alle Aenderungen als Mod-Archiv – entweder als Mod fuer den
// Mod Starter (FanMods\<Ordner>\Projects_compiled.m0x bzw. .p0x) oder direkt nach Data.
func (a *app) save() {
	if a.cpSaveBlocked() {
		return
	}
	if !a.commitDialog() || !a.commitQuest() {
		return
	}
	a.fixMarkers()
	if !a.ed.dirty {
		a.flash(T("Keine Änderungen zu speichern."))
		return
	}
	if !a.showModCheck(true) {
		return
	}
	mode, mds := a.saveMode()
	if mode == "" {
		return
	}
	var target, modDir string
	if mode == "data" && activeModFolder() == "" && loadConfig()["ModFile"] != "" {
		messageBox(a.hwnd, T("Du arbeitest an der Originalwelt. Eine zweite, getrennte Mod geht nur mit dem Mod Starter.\n\nStelle dafür in AdanosEdit.ini SaveMode=mds ein oder wähle oben wieder deine Mod aus."), T(appTitleDE), MB_ICONINFORMATION)
		return
	}
	if mode == "mds" {
		modDir = a.mdsModDir(mds)
		if modDir == "" {
			name := pendingModName()
			if name == "" {
				var ok bool
				name, ok = a.askModName(T("Neue Mod"), T("Name der neuen Mod (so erscheint sie im Mod Starter):"), "")
				if !ok {
					return
				}
			}
			folder := modFolderName(mds, name)
			modDir = filepath.Join(mds, "FanMods", folder)
			if err := os.MkdirAll(modDir, 0755); err != nil {
				messageBox(a.hwnd, fmt.Sprintf(T("Der Mod-Ordner konnte nicht angelegt werden:\n%s"), err.Error()), T(appTitleDE), MB_ICONERROR)
				return
			}
			writeModName(modDir, name)
			code := loadConfig()["MDSNewModCode"]
			if code == "" {
				code = a.uniqueModCode(name)
			}
			writeModMarker(modDir, code)
			cfg := loadConfig()
			cfg["MDSModFolder"] = folder
			delete(cfg, "MDSNewModName")
			delete(cfg, "MDSNewModCode")
			saveConfig(cfg)
			a.modList = nil
			defer a.fillModList()
		}
		if err := os.MkdirAll(modDir, 0755); err != nil {
			messageBox(a.hwnd, fmt.Sprintf(T("Der Mod-Ordner konnte nicht angelegt werden:\n%s"), err.Error()), T(appTitleDE), MB_ICONERROR)
			return
		}
		target = a.modArchive(modDir, "Projects_compiled")
	} else {
		target = a.modTarget()
	}
	files := map[string][]byte{}
	addAll := func(ar *g3.Pak) {
		for _, e := range ar.Entries {
			if d, err := e.Read(); err == nil {
				files[e.Name] = d
			}
		}
	}
	// Bisherige eigene Datei(en) bleiben erhalten. Beim Wechsel zum Mod Starter
	// wird eine fruehere Projects_compiled.mod aus Data uebernommen.
	var oldPak, migrate *g3.Pak
	if mode == "mds" {
		cfg := loadConfig()
		if m := cfg["ModFile"]; m != "" {
			mp := filepath.Join(a.gothicDir, "Data", m)
			for _, ar := range a.vfs.Archives {
				if strings.EqualFold(ar.Path, mp) {
					migrate = ar
					addAll(ar)
				}
			}
		}
	}
	for _, ar := range a.vfs.Archives {
		if strings.EqualFold(ar.Path, target) {
			oldPak = ar
			addAll(ar)
		}
	}
	n := 0
	for path, wf := range a.ed.files {
		if !wf.Dirty {
			continue
		}
		files[path] = wf.Bytes()
		n++
		if strings.HasSuffix(strings.ToLower(path), ".node") {
			geo := path[:len(path)-5] + ".lrgeodat"
			if e := a.vfs.Find("Projects_compiled", geo); e != nil {
				if gd, err := e.Read(); err == nil {
					files[e.Name] = wf.UpdateGeoContext(gd)
				}
			}
		}
	}
	// Navigationskarte: nur, wenn die Welt eigene Zonen enthaelt (sonst entfernen)
	for k := range files {
		if strings.EqualFold(k, g3.NavMapPath) {
			delete(files, k)
		}
	}
	nav, navNotes := a.buildModNavMap()
	if nav != nil {
		files[g3.NavMapPath] = nav
	}
	var list []g3.PakFile
	var names []string
	for k := range files {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		list = append(list, g3.PakFile{Name: k, Data: files[k]})
	}
	// Vom Mod Starter bereits integrierte Kopie finden (vor dem Ueberschreiben)
	integrated := ""
	if mode == "mds" {
		if old, err := os.ReadFile(target); err == nil {
			integrated = a.integratedCopy(old)
		}
		a.shotReq = true
		a.frame(0, time.Now())
	}
	if oldPak != nil {
		oldPak.Close()
	}
	err := g3.WritePak(target, list)
	if err == nil {
		err = a.vfs.Reopen(target, oldPak)
	} else if oldPak != nil {
		// alte Fassung wieder einbinden, damit weitergearbeitet werden kann
		if e := a.vfs.Reopen(target, oldPak); e != nil {
			log.Println("Wiedereinbinden:", e)
		}
	}
	// Sicherungskopie neben dem Programm
	bdir := filepath.Join(exeDir(), "Mod-Sicherungen")
	os.MkdirAll(bdir, 0755)
	bname := filepath.Base(target)
	if mode == "mds" {
		bname = "ModStarter_" + bname
	}
	backup := filepath.Join(bdir, time.Now().Format("2006-01-02_15-04-05")+"_"+bname)
	if err2 := g3.WritePak(backup, list); err2 != nil {
		log.Println("Sicherung:", err2)
	}
	if err != nil {
		log.Println("Speichern:", err)
		messageBox(a.hwnd, fmt.Sprintf(T("Die Mod-Datei konnte nicht geschrieben werden:\n%s\n\nEine Kopie liegt hier:\n%s\n\nDu kannst sie von Hand nach\n%s\nkopieren (eventuell mit Administratorrechten)."), err.Error(), backup, filepath.Dir(target)), T(appTitleDE), MB_ICONERROR)
		return
	}
	notes := navNotes
	if nav != nil {
		if others := a.otherNavMapMods(target, integrated); len(others) > 0 {
			notes = append(notes, fmt.Sprintf(T("Achtung: Im Spiel ist schon eine andere Mod mit eigener Navigationskarte aktiv (%s). Es gilt immer nur eine Karte – die Zonen der anderen Mod oder deine eigenen werden dann nicht erkannt. Am besten nur eine solche Mod gleichzeitig integrieren."), strings.Join(others, ", ")))
		}
	}
	if mode == "mds" {
		// weitere Archive (Texte, Vorlagen, Dialoge ...)
		newArchive := false
		for cat, fl := range a.ed.extra {
			if len(fl) == 0 {
				continue
			}
			et := a.modArchive(modDir, cat)
			ef := map[string][]byte{}
			var old *g3.Pak
			for _, ar := range a.vfs.Archives {
				if strings.EqualFold(ar.Path, et) {
					old = ar
					for _, e := range ar.Entries {
						if d, err := e.Read(); err == nil {
							ef[e.Name] = d
						}
					}
				}
			}
			for k, d := range fl {
				if d == nil {
					// geloescht (gleich, ob Gross-/Kleinschreibung abweicht)
					for ok := range ef {
						if strings.EqualFold(ok, k) {
							delete(ef, ok)
						}
					}
					continue
				}
				for ok := range ef {
					if ok != k && strings.EqualFold(ok, k) {
						delete(ef, ok)
					}
				}
				ef[k] = d
			}
			if len(ef) == 0 && old == nil {
				delete(a.ed.extra, cat)
				continue
			}
			var el []g3.PakFile
			for k, d := range ef {
				el = append(el, g3.PakFile{Name: k, Data: d})
			}
			sort.Slice(el, func(i, j int) bool { return el[i].Name < el[j].Name })
			integ := ""
			if ob, e := os.ReadFile(et); e == nil {
				integ = a.integratedCopyOf(ob, cat)
			}
			if old != nil {
				old.Close()
			}
			e1 := g3.WritePak(et, el)
			if e1 == nil {
				e1 = a.vfs.Reopen(et, old)
			} else if old != nil {
				a.vfs.Reopen(et, old)
			}
			g3.WritePak(filepath.Join(bdir, time.Now().Format("2006-01-02_15-04-05")+"_ModStarter_"+cat+a.modExt()), el)
			if e1 != nil {
				notes = append(notes, fmt.Sprintf(T("Achtung: %s konnte nicht geschrieben werden (%s)."), filepath.Base(et), e1.Error()))
				continue
			}
			if integ != "" {
				if b, e := os.ReadFile(et); e == nil {
					os.WriteFile(integ, b, 0644)
				}
			} else {
				newArchive = true
			}
			delete(a.ed.extra, cat)
		}
		// Texte aus frueheren Versionen (Strings.p0x) werden nicht mehr gebraucht
		for _, sp := range []string{filepath.Join(modDir, "Strings.p0x"), filepath.Join(modDir, "Strings.m0x")} {
			if !fileExists(sp) {
				continue
			}
			for _, ar := range a.vfs.Archives {
				if strings.EqualFold(ar.Path, sp) {
					a.vfs.Drop(ar)
					break
				}
			}
			os.Remove(sp)
		}
		a.writeMDSExtras(modDir, n)
		// Fruehere Data\Projects_compiled.mod abschalten (waere sonst immer aktiv)
		if migrate != nil {
			a.vfs.Drop(migrate)
			off := migrate.Path + ".vor-ModStarter"
			if e := os.Rename(migrate.Path, off); e != nil {
				notes = append(notes, fmt.Sprintf(T("Achtung: Die alte Datei\n%s\nkonnte nicht umbenannt werden (%s).\nSie ist weiterhin immer aktiv – bitte von Hand umbenennen oder löschen."), migrate.Path, e.Error()))
			} else {
				notes = append(notes, fmt.Sprintf(T("Die frühere Datei %s im Data-Ordner wurde übernommen und in\n%s umbenannt (damit abgeschaltet)."), filepath.Base(migrate.Path), filepath.Base(off)))
				cfg := loadConfig()
				delete(cfg, "ModFile")
				saveConfig(cfg)
			}
		}
		// Ist die Mod schon integriert, die Kopie im Data-Ordner gleich mitaktualisieren
		if integrated != "" && newArchive {
			notes = append(notes, T("Die Mod enthält jetzt weitere Archive (z. B. Texte für neue NPCs). Bitte im Mod Starter einmal neu integrieren."))
		} else if integrated != "" {
			if b, e := os.ReadFile(target); e == nil {
				if e := os.WriteFile(integrated, b, 0644); e == nil {
					notes = append(notes, fmt.Sprintf(T("Die Mod ist im Mod Starter bereits integriert – die Kopie\n%s\nwurde gleich mit aktualisiert. Neu integrieren ist nicht nötig."), integrated))
				} else {
					notes = append(notes, fmt.Sprintf(T("Die integrierte Kopie %s konnte nicht aktualisiert werden (%s). Bitte im Mod Starter neu integrieren."), integrated, e.Error()))
				}
			}
		} else {
			notes = append(notes, fmt.Sprintf(T("Zum Spielen: Mod Starter (mds.exe) öffnen, „%s“ anhaken – zusammen mit allen anderen Mods, die aktiv bleiben sollen – und „Integrate Mods“ klicken.\nZum Abschalten: abwählen und erneut integrieren."), filepath.Base(modDir)))
		}
	} else {
		cfg := loadConfig()
		cfg["ModFile"] = filepath.Base(target)
		saveConfig(cfg)
		notes = append(notes, T("Zum Rückgängigmachen die Datei einfach löschen oder umbenennen."))
	}
	// umbenannte Truhen (fruehere Versionen): Anzeigenamen im Spiel ergaenzen
	if a.world != nil {
		for i := range a.world.Inst {
			e := a.world.Inst[i].Ent
			if !a.world.Inst[i].Hidden && strings.HasPrefix(e.Name, "G3V_") && e.Kind != g3.KindNPC && hasClass(e, "gCInventory_PS") {
				a.keepFocusName(e.Name, "")
			}
		}
	}
	var pending []string
	for k := range a.ed.strs {
		pending = append(pending, k)
	}
	// Texte ins Spiel nur, wenn genau diese Mod dort integriert ist (sonst
	// stuenden sie auch bei anderen Mods); ansonsten traegt der Mod Starter
	// sie beim Integrieren aus der stringtableMod.ini ein.
	if note := a.saveStrings(mode == "data" || integrated != ""); note != "" {
		notes = append(notes, note)
	}
	if mode == "mds" && modDir != "" {
		if note := a.writeStringtableMod(modDir, pending); note != "" {
			notes = append(notes, note)
		}
		if integrated == "" && len(pending) > 0 {
			notes = append(notes, T("Neue Texte (Namen, Dialoge, Quest-Titel) stehen in der stringtableMod.ini der Mod und kommen beim Integrieren im Mod Starter ins Spiel."))
		}
		a.ed.strs = nil
	}
	for _, wf := range a.ed.files {
		wf.Dirty = false
	}
	a.ed.dirty = false
	a.updateTitle()
	log.Printf("Gespeichert: %s (%d Weltdateien neu, %d gesamt)", target, n, len(list))
	head := fmt.Sprintf(T("Gespeichert als Mod:\n%s"), target)
	if mode == "mds" {
		head = fmt.Sprintf(T("Gespeichert als Mod für den Mod Starter:\n%s"), modDir)
	}
	messageBox(a.hwnd, fmt.Sprintf(T("%s\n\n%d Weltdatei(en) geändert, insgesamt %d Datei(en) in der Mod.\n\n%s\n\nDie Änderungen wirken in einem neuen Spiel (alte Spielstände enthalten die Welt schon)."),
		head, n, len(list), strings.Join(notes, "\n\n")), T(appTitleDE), MB_ICONINFORMATION)
}

func (a *app) flash(s string) {
	a.flashText = s
	a.flashUntil = time.Now().Add(5 * time.Second)
	a.updateStatus()
}

// planeHit schneidet den Mausstrahl mit der Ebene y = konstant.
func (a *app) planeHit(p POINT, y float32) (scene.Vec3, bool) {
	o, d := a.cam.Ray(float32(p.X), float32(p.Y), float32(a.width), float32(a.height))
	if d[1] > -1e-4 && d[1] < 1e-4 {
		return scene.Vec3{}, false
	}
	t := (y - o[1]) / d[1]
	if t < 0 || t > 200000 {
		return scene.Vec3{}, false
	}
	return o.Add(d.Scale(t)), true
}

// centerPoint liefert den Weltpunkt in der Bildmitte (aus dem letzten Bild).
func (a *app) centerPoint() (scene.Vec3, bool) {
	return a.lastCenter, a.centerValid
}

// dropSelected setzt das ausgewaehlte Objekt auf die darunterliegende
// Weltgeometrie (Gelaende, Boeden, Felsen ...).
func (a *app) dropSelected() {
	if a.inGroup() {
		a.dropGroup()
		return
	}
	in, _, ok := a.selectedEditable()
	if !ok || a.loader == nil {
		return
	}
	setText(a.hStatus, T("Boden wird gesucht …"))
	dy, found := a.groundOffset(a.selected)
	a.lastStatusText = ""
	if !found {
		a.flash(T("Unter dem Objekt wurde kein Boden gefunden."))
		return
	}
	w := in.Ent.World
	w[13] += dy
	a.setSelectedWorld(w, true)
	a.flash(fmt.Sprintf(T("Objekt auf den Boden gesetzt (%+.0f cm)."), dy))
}

// groundOffset: wie weit ein Objekt sinken/steigen muss, um auf dem Boden zu stehen.
func (a *app) groundOffset(id int32) (float32, bool) {
	in := &a.world.Inst[id]
	mn, mx := in.Ent.NodeMin, in.Ent.NodeMax
	if !g3.BoxValid(mn, mx) {
		mn, mx = in.Min, in.Max
	}
	cx, cz := (mn[0]+mx[0])/2, (mn[2]+mx[2])/2
	hx, hz := (mx[0]-mn[0])*0.3, (mx[2]-mn[2])*0.3
	pts := [][2]float32{{cx, cz}, {cx - hx, cz - hz}, {cx + hx, cz - hz}, {cx - hx, cz + hz}, {cx + hx, cz + hz}}
	try := func(startY float32) (float32, bool) {
		best, found := float32(0), false
		for _, p := range pts {
			if y, ok := a.world.GroundBelow(p[0], startY, p[1], id, a.loader.Geometry); ok && (!found || y > best) {
				best, found = y, true
			}
		}
		return best, found
	}
	start := (mn[1] + mx[1]) / 2
	if start < mn[1]+30 {
		start = mn[1] + 30
	}
	ground, found := try(start)
	if !found {
		ground, found = try(mx[1] + 500)
	}
	if !found {
		return 0, false
	}
	return ground - mn[1], true
}

// clipEntry merkt sich ein mit Strg+C kopiertes Objekt.
type clipEntry struct {
	wf     *g3.WorldFile
	idx    int
	sector string
	name   string
}

// copySelected merkt sich das ausgewaehlte Objekt fuer Strg+V.
func (a *app) copySelected() {
	if a.groupOnlyOne() {
		return
	}
	in, wf, ok := a.selectedEditable()
	if !ok {
		return
	}
	if in.Ent.Index == 0 {
		a.flash(T("die Wurzel kann nicht kopiert werden"))
		return
	}
	name := in.Ent.Name
	if name == "" {
		name = in.Ent.Mesh
	}
	a.clip = &clipEntry{wf: wf, idx: in.Ent.Index, sector: in.Sector, name: name}
	a.flash(fmt.Sprintf(T("„%s“ kopiert. Strg+V fügt es in der Bildmitte ein."), name))
}

// pasteClip fuegt das kopierte Objekt in der Bildmitte ein.
func (a *app) pasteClip() {
	c := a.clip
	if c == nil || a.world == nil {
		a.flash(T("Nichts zum Einfügen – zuerst ein Objekt auswählen und Strg+C drücken."))
		return
	}
	wf := c.wf
	ci, err := wf.Clone(c.idx)
	if err != nil {
		a.flash(T(err.Error()))
		return
	}
	ne := wf.Ents[ci]
	w := ne.World
	if p, ok := a.centerPoint(); ok {
		// Unterkante auf die getroffene Flaeche setzen
		off := float32(0)
		if g3.BoxValid(ne.NodeMin, ne.NodeMax) {
			off = w[13] - ne.NodeMin[1]
		}
		w[12], w[13], w[14] = p[0], p[1]+off, p[2]
	} else {
		f := a.cam.Forward()
		q := a.cam.Pos.Add(f.Scale(400))
		w[12], w[13], w[14] = q[0], q[1], q[2]
	}
	wf.SetWorld(ci, w)
	ii := a.world.Add(wf.Ents[ci], c.sector)
	if ii < 0 {
		wf.Delete(ci)
		a.syncFile(wf)
		a.flash(T("Die Kopie konnte nicht angelegt werden."))
		return
	}
	if a.ed.inst[wf.Path] == nil {
		a.ed.inst[wf.Path] = map[int]int32{}
	}
	a.ed.inst[wf.Path][ci] = ii
	a.syncFile(wf)
	a.ed.undo = append(a.ed.undo, func() {
		wf.Delete(ci)
		a.syncFile(wf)
		if a.selected == ii {
			a.select_(-1, false)
		}
	})
	a.select_(ii, false)
	a.flash(fmt.Sprintf(T("„%s“ eingefügt. Umschalt+F setzt es auf den Boden, Strg+Z entfernt es wieder."), c.name))
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// stringtablePath ist die vom Community Patch verwendete lose Texttabelle.
func (a *app) stringtablePath() string {
	return filepath.Join(a.gothicDir, "Data", "Strings", "stringtable.ini")
}

// saveStrings traegt neue Texte (z. B. Namen neuer NPCs) in die Texttabelle
// des Spiels ein. Der Community Patch liest Data\Strings\stringtable.ini
// direkt (nicht aus den Archiven), deshalb wird diese Datei ergaenzt. Vorher
// wird einmalig eine Sicherung angelegt. Neue Eintraege stoeren das Spiel
// auch dann nicht, wenn die Mod abgewaehlt wird.
func (a *app) saveStrings(toGame bool) string {
	if len(a.ed.strs) == 0 {
		return ""
	}
	if !toGame {
		return ""
	}
	apply := func(d []byte) []byte {
		keys := make([]string, 0, len(a.ed.strs))
		for k := range a.ed.strs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			d = g3.SetStringEntry(d, k, a.ed.strs[k])
		}
		return d
	}
	fail := func(p string, err error) string {
		return fmt.Sprintf(T("Achtung: %s konnte nicht geschrieben werden (%s)."), p, err.Error())
	}
	// 1. lose Datei Data\Strings\stringtable.ini (hat Vorrang vor den Archiven)
	p := a.stringtablePath()
	if d, err := os.ReadFile(p); err == nil {
		bak := p + ".vor-G3Viewer"
		if !fileExists(bak) {
			os.WriteFile(bak, d, 0644)
		}
		if err := os.WriteFile(p, apply(d), 0644); err != nil {
			return fail(p, err)
		}
		a.ed.strs = nil
		// zwischengespeicherten Auszug beiseitelegen, damit das Spiel ihn neu erzeugt
		bin := filepath.Join(filepath.Dir(p), "_stringtable.bin")
		if fileExists(bin) {
			os.Remove(bin + ".alt")
			os.Rename(bin, bin+".alt")
		}
		return ""
	}
	// 2. vom Mod Starter erzeugtes Archiv (Strings.p00 / .m00 ...): dort ergaenzen.
	//    Beim naechsten Integrieren erzeugt der Mod Starter es aus der
	//    stringtableMod.ini der Mod ohnehin neu.
	if ar := a.stringsGeneration(); ar != nil {
		var files []g3.PakFile
		found := false
		for _, e := range ar.Entries {
			d, err := e.Read()
			if err != nil {
				return fail(ar.Path, err)
			}
			if strings.EqualFold(e.Name, "stringtable.ini") {
				d = apply(d)
				found = true
			}
			files = append(files, g3.PakFile{Name: e.Name, Data: d})
		}
		if found {
			ar.Close()
			err := g3.WritePak(ar.Path, files)
			if e := a.vfs.Reopen(ar.Path, ar); e != nil && err == nil {
				err = e
			}
			if err != nil {
				return fail(ar.Path, err)
			}
			a.ed.strs = nil
			return ""
		}
	}
	// 3. nur das Original-Archiv: lose Datei anlegen (wie es der Mod Starter tut)
	d := a.currentStringtable()
	if d == nil {
		return fail(p, fmt.Errorf("stringtable.ini"))
	}
	os.MkdirAll(filepath.Dir(p), 0755)
	if err := os.WriteFile(p, apply(d), 0644); err != nil {
		return fail(p, err)
	}
	a.ed.strs = nil
	return ""
}

// stringsGeneration liefert das oberste vom Mod Starter erzeugte Texte-Archiv
// (Data\Strings.p00 bzw. Strings.m00 ...), falls vorhanden. Die .m-Generationen
// laedt das Spiel nach den .p-Generationen, die Reihenfolge im VFS stimmt.
func (a *app) stringsGeneration() *g3.Pak {
	var top *g3.Pak
	re := regexp.MustCompile(`(?i)^strings\.[pm]\d\d$`)
	for _, ar := range a.vfs.Archives {
		if re.MatchString(filepath.Base(ar.Path)) && strings.EqualFold(filepath.Dir(ar.Path), a.vfs.DataDir) {
			top = ar
		}
	}
	return top
}

// baseStringtable liefert die Texttabelle aus dem Original-Archiv Strings.pak.
func (a *app) baseStringtable() []byte {
	for _, ar := range a.vfs.Archives {
		if !strings.EqualFold(filepath.Base(ar.Path), "Strings.pak") {
			continue
		}
		for _, e := range ar.Entries {
			if strings.EqualFold(e.Name, "stringtable.ini") {
				if d, err := e.Read(); err == nil {
					return d
				}
			}
		}
	}
	return nil
}

// writeStringtableMod legt die Texte der Mod (Namen, Dialoge, Quests) als
// stringtableMod.ini in den Mod-Ordner. Der Mod Starter traegt sie beim
// Integrieren in die stringtable.ini des Spielers ein - so kommen sie auch
// bei anderen an, die die Mod installieren.
func (a *app) writeStringtableMod(modDir string, pending []string) string {
	p := filepath.Join(modDir, "stringtableMod.ini")
	keys := map[string]bool{}
	oldVals := map[string]string{}
	if d, err := os.ReadFile(p); err == nil {
		oldVals = g3.StringValues(d)
		for k := range oldVals {
			keys[k] = true
		}
	}
	for _, k := range pending {
		keys[k] = true
	}
	// Name des Helden nie mitliefern (frueher versehentlich moeglich)
	for k := range keys {
		if strings.EqualFold(k, "FO_PC_Hero") {
			delete(keys, k)
		}
	}
	curData := a.currentStringtable()
	if curData == nil {
		return ""
	}
	cur := g3.StringValues(curData)
	orig := g3.StringValues(a.baseStringtable())
	added := func(k string) bool {
		v, ok := cur[k]
		if !ok {
			return false
		}
		o, ok := orig[k]
		return !ok || o != v
	}
	// Texte, die Dialoge und Quests der Mod verwenden (auch aus aelteren Versionen)
	for _, cat := range []string{"Infos", "Quests"} {
		ap := a.modArchive(modDir, cat)
		for _, ar := range a.vfs.Archives {
			if !strings.EqualFold(ar.Path, ap) {
				continue
			}
			for _, e := range ar.Entries {
				d, err := e.Read()
				if err != nil {
					continue
				}
				if cat == "Infos" {
					for _, c := range g3.ParseInfo(d).Script {
						if c.Text != "" && added(c.Text) {
							keys[c.Text] = true
						}
					}
				} else if k := g3.ParseQuest(d).Get("LogTopic"); k != "" && added(k) {
					keys[k] = true
				}
			}
		}
	}
	// Namen neuer NPCs dieser Mod
	for k := range cur {
		if strings.HasPrefix(k, "FO_") && added(k) && a.worldHasName(k[3:]) {
			if _, inOrig := orig[k]; !inOrig {
				keys[k] = true
			}
		}
	}
	vals := map[string]string{}
	edited := map[string]bool{}
	for _, k := range pending {
		edited[k] = true
	}
	for k := range keys {
		// eigener Text der Mod hat Vorrang vor der Texttabelle des Spiels
		// (dort kann eine andere integrierte Mod denselben Schluessel haben);
		// nur in dieser Sitzung geaenderte Texte kommen aus der Tabelle
		if v, ok := oldVals[k]; ok && !edited[k] {
			vals[k] = v
		} else if t, ok := a.ed.strs[k]; ok && edited[k] {
			// in dieser Sitzung geaendert (steht evtl. nicht im Spiel)
			vals[k] = g3.StringValue(t)
		} else if v, ok := cur[k]; ok {
			vals[k] = v
		}
	}
	if len(vals) == 0 {
		return ""
	}
	if err := os.WriteFile(p, g3.StringtableMod(vals), 0644); err != nil {
		return fmt.Sprintf(T("Achtung: %s konnte nicht geschrieben werden (%s)."), p, err.Error())
	}
	return ""
}

// ensureStringtableMod legt die stringtableMod.ini fuer Mods aus aelteren
// Versionen nachtraeglich an (einmalig beim Laden).
func (a *app) ensureStringtableMod() {
	mds := a.findMDS()
	if mds == "" {
		return
	}
	dir := a.mdsModDir(mds)
	if dir == "" || fileExists(filepath.Join(dir, "stringtableMod.ini")) {
		return
	}
	if !fileExists(a.modArchive(dir, "Infos")) && !fileExists(a.modArchive(dir, "Quests")) && !fileExists(filepath.Join(dir, modMarker)) {
		return
	}
	a.writeStringtableMod(dir, nil)
}
