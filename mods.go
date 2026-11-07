//go:build windows

package main

// Mehrere eigene Mods: Auswahl, welche Mod bearbeitet wird, und
// "Originalwelt" als Ausgangspunkt fuer eine neue Mod.

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"unicode"
	"unsafe"

	"g3viewer/g3"
)

// Kennzeichnet einen Mod-Ordner als vom Viewer angelegt.
const modMarker = "g3viewer.id"

// newModValue in AdanosEdit.ini (MDSModFolder): Originalwelt, neue Mod beim Speichern.
const newModValue = "-"

type modInfo struct {
	folder  string // Ordnername in FanMods
	caption string
	dir     string
}

// activeModFolder liefert den Ordner der bearbeiteten Mod ("" = neue Mod).
func activeModFolder() string {
	f := loadConfig()["MDSModFolder"]
	switch f {
	case newModValue:
		return ""
	case "":
		return mdsDefaultFolder
	}
	return f
}

// modCaption liest den Anzeigenamen aus ModName.ini.
func modCaption(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "ModName.ini"))
	if err != nil {
		return ""
	}
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		b = b[3:]
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "caption=") {
			c := line[8:]
			// Windows-1252 -> UTF-8 (nur Latin-1-Bereich)
			if !isUTF8(c) {
				var r []rune
				for i := 0; i < len(c); i++ {
					switch c[i] {
					case 0x96:
						r = append(r, '–')
					case 0x97:
						r = append(r, '—')
					default:
						r = append(r, rune(c[i]))
					}
				}
				c = string(r)
			}
			return c
		}
	}
	return ""
}

func isUTF8(s string) bool {
	for _, r := range s {
		if r == unicode.ReplacementChar {
			return false
		}
	}
	return true
}

// viewerMods listet die vom Viewer angelegten Mods im Mod Starter.
func (a *app) viewerMods() []modInfo {
	mds := a.findMDS()
	if mds == "" {
		return nil
	}
	root := filepath.Join(mds, "FanMods")
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []modInfo
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		_, errM := os.Stat(filepath.Join(dir, modMarker))
		_, errP := os.Stat(a.modArchive(dir, "Projects_compiled"))
		if errP != nil || (errM != nil && !strings.EqualFold(e.Name(), mdsDefaultFolder)) {
			continue
		}
		c := modCaption(dir)
		if c == "" {
			c = e.Name()
		}
		out = append(out, modInfo{folder: e.Name(), caption: c, dir: dir})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].caption) < strings.ToLower(out[j].caption) })
	return out
}

// dropOtherMods entfernt beim Start die bereits ins Spiel integrierten Kopien
// der eigenen Mods (und im Data-Modus die eigene .mod-Datei), damit die
// Grundlage die Originalwelt ist. Die bearbeitete Mod wird danach als
// oberste Schicht geladen.
func (a *app) dropOtherMods(v *g3.VFS) {
	var sizes []int64
	var paths []string
	for _, m := range a.viewerMods() {
		ps, _ := filepath.Glob(filepath.Join(m.dir, "*"+a.modExt()))
		for _, p := range ps {
			if st, err := os.Stat(p); err == nil {
				sizes = append(sizes, st.Size())
				paths = append(paths, p)
			}
		}
	}
	cfg := loadConfig()
	// Vom Mod Starter integrierte Mods (fdm.dat) gehoeren nicht zur Grundlage:
	// Eigene Mods werden auf dem unveraenderten Spiel (mit Community Patch)
	// aufgebaut; die gerade bearbeitete Mod laedt AdanosEdit aus FanMods.
	managed := map[string]bool{}
	if mds := a.findMDS(); mds != "" && cfg["SaveMode"] != "data" {
		if b, err := os.ReadFile(filepath.Join(mds, "fdm.dat")); err == nil {
			for _, l := range strings.Split(string(b), "\n") {
				if l = strings.TrimSpace(l); l != "" {
					managed[strings.ToLower(filepath.Base(l))] = true
				}
			}
		}
	}
	for _, ar := range append([]*g3.Pak(nil), v.Archives...) {
		if managed[strings.ToLower(filepath.Base(ar.Path))] && strings.EqualFold(filepath.Dir(ar.Path), v.DataDir) {
			log.Println("Vom Mod Starter integriert, ausgeblendet:", filepath.Base(ar.Path))
			v.Drop(ar)
		}
	}
	gen := regexp.MustCompile(`(?i)\.(p\d\d|m\d\d|mod)$`)
	for _, ar := range append([]*g3.Pak(nil), v.Archives...) {
		base := filepath.Base(ar.Path)
		if !gen.MatchString(base) {
			continue
		}
		if !strings.EqualFold(filepath.Dir(ar.Path), v.DataDir) {
			continue
		}
		// eigene Mod im Data-Modus
		if m := cfg["ModFile"]; m != "" && strings.EqualFold(base, m) && activeModFolder() == "" {
			log.Println("Originalwelt: blende aus", base)
			v.Drop(ar)
			continue
		}
		st, err := os.Stat(ar.Path)
		if err != nil {
			continue
		}
		bn := strings.ToLower(strings.TrimSuffix(base, filepath.Ext(base)))
		for k, s := range sizes {
			pb := strings.ToLower(strings.TrimSuffix(filepath.Base(paths[k]), filepath.Ext(paths[k])))
			if s != st.Size() || pb != bn {
				continue
			}
			if sameFile(ar.Path, paths[k]) {
				log.Println("Eigene Mod im Data-Ordner ausgeblendet:", base)
				v.Drop(ar)
				break
			}
		}
	}
}

func sameFile(a, b string) bool {
	x, err := os.ReadFile(a)
	if err != nil {
		return false
	}
	y, err := os.ReadFile(b)
	if err != nil {
		return false
	}
	return bytes.Equal(x, y)
}

// modFolderName bildet aus einem Anzeigenamen einen gueltigen, freien Ordnernamen.
func modFolderName(mdsDir, name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('_')
		case r == 'ä':
			b.WriteString("ae")
		case r == 'ö':
			b.WriteString("oe")
		case r == 'ü':
			b.WriteString("ue")
		case r == 'Ä':
			b.WriteString("Ae")
		case r == 'Ö':
			b.WriteString("Oe")
		case r == 'Ü':
			b.WriteString("Ue")
		case r == 'ß':
			b.WriteString("ss")
		}
	}
	base := strings.Trim(b.String(), "_-")
	if base == "" {
		base = "Mod"
	}
	base = "AdanosEdit_" + base
	f := base
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(mdsDir, "FanMods", f)); os.IsNotExist(err) {
			return f
		}
		f = fmt.Sprintf("%s_%d", base, i)
	}
}

// fillModList fuellt das Auswahlfeld der Mods.
func (a *app) fillModList() {
	sendMessage(a.hMod, CB_RESETCONTENT, 0, 0)
	a.modList = a.viewerMods()
	first := T("Originalwelt (neue Mod anlegen)")
	if n := pendingModName(); n != "" {
		first = fmt.Sprintf(T("Neue Mod: %s (noch nicht gespeichert)"), n)
	}
	sendMessage(a.hMod, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(first))))
	sel := 0
	act := activeModFolder()
	found := act == ""
	for i, m := range a.modList {
		sendMessage(a.hMod, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(m.caption))))
		if strings.EqualFold(m.folder, act) {
			sel = i + 1
			found = true
		}
	}
	a.modRename = found && act != ""
	if a.modRename {
		sendMessage(a.hMod, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(T("Mod umbenennen …")))))
	}
	sendMessage(a.hMod, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(T("Mod Starter wählen …")))))
	if !found {
		// noch nie gespeicherte Standard-Mod: wie Originalwelt behandeln
		sel = 0
	}
	sendMessage(a.hMod, CB_SETCURSEL, uintptr(sel), 0)
	a.modSel = sel
}

// onModSelect wechselt die bearbeitete Mod (Neustart des Programms).
func (a *app) onModSelect() {
	i := int(sendMessage(a.hMod, CB_GETCURSEL, 0, 0))
	if i < 0 || i == a.modSel {
		return
	}
	var folder, caption, newName string
	if i == 0 {
		// Name der neuen Mod gleich festlegen; Abbrechen = Originalwelt nur ansehen
		folder, caption = newModValue, T("Originalwelt")
		name, ok := a.askModName(T("Neue Mod"), T("Name der neuen Mod (so erscheint sie im Mod Starter).\nAbbrechen = nur die Originalwelt ansehen, ohne neue Mod."), "")
		newCode := ""
		if ok {
			newName = name
			caption = name
			newCode = a.uniqueModCode(name)
		}
		if activeModFolder() == "" {
			// schon in der Originalwelt: nur den Namen merken, kein Neustart
			cfg := loadConfig()
			cfg["MDSNewModName"] = newName
			cfg["MDSNewModCode"] = newCode
			saveConfig(cfg)
			a.fillModList()
			a.updateTitle()
			return
		}
	} else if i-1 < len(a.modList) {
		folder, caption = a.modList[i-1].folder, a.modList[i-1].caption
	} else if a.modRename && i-1 == len(a.modList) {
		sendMessage(a.hMod, CB_SETCURSEL, uintptr(a.modSel), 0)
		a.renameMod()
		return
	} else {
		sendMessage(a.hMod, CB_SETCURSEL, uintptr(a.modSel), 0)
		a.chooseMDS()
		return
	}
	if a.ed.dirty {
		r := messageBoxRet(a.hwnd, T("Es gibt ungespeicherte Änderungen.\n\nVor dem Wechsel als Mod speichern?"), T(appTitleDE), MB_YESNOCANCEL|MB_ICONWARNING)
		if r == IDCANCEL {
			sendMessage(a.hMod, CB_SETCURSEL, uintptr(a.modSel), 0)
			return
		}
		if r == IDYES {
			a.save()
			if a.ed.dirty {
				sendMessage(a.hMod, CB_SETCURSEL, uintptr(a.modSel), 0)
				return
			}
		}
	} else if messageBoxRet(a.hwnd, fmt.Sprintf(T("Zu „%s“ wechseln?\n\nDer Viewer startet dafür neu und lädt die Welt neu."), caption), T(appTitleDE), MB_YESNO|MB_ICONINFORMATION) != IDYES {
		sendMessage(a.hMod, CB_SETCURSEL, uintptr(a.modSel), 0)
		return
	}
	cfg := loadConfig()
	cfg["MDSModFolder"] = folder
	cfg["MDSNewModName"] = newName
	cfg["MDSNewModCode"] = ""
	if newName != "" {
		cfg["MDSNewModCode"] = a.uniqueModCode(newName)
	}
	cfg["ViewDist"] = fmt.Sprintf("%.0f", a.viewDist)
	cfg["CamPos"] = fmt.Sprintf("%.0f %.0f %.0f %.4f %.4f", a.cam.Pos[0], a.cam.Pos[1], a.cam.Pos[2], a.cam.Yaw, a.cam.Pitch)
	saveConfig(cfg)
	a.restart()
}

// restart startet das Programm neu.
func (a *app) restart() {
	if instanceMutex != 0 {
		pCloseHandle.Call(instanceMutex) // damit das neue Fenster starten darf
		instanceMutex = 0
	}
	exe, err := os.Executable()
	if err == nil {
		cmd := exec.Command(exe)
		cmd.Dir = filepath.Dir(exe)
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		err = cmd.Start()
	}
	if err != nil {
		messageBox(a.hwnd, fmt.Sprintf(T("Neustart nicht möglich: %s\nBitte den Viewer von Hand neu starten."), err.Error()), T(appTitleDE), MB_ICONERROR)
	}
	a.shutdown()
}

// modTitle liefert den Namen der bearbeiteten Mod fuer die Titelzeile.
func (a *app) modTitle() string {
	act := activeModFolder()
	if act == "" {
		if n := pendingModName(); n != "" {
			return fmt.Sprintf(T("Neue Mod: %s"), n)
		}
		return T("Originalwelt")
	}
	if a.modList == nil {
		a.modList = a.viewerMods()
	}
	for _, m := range a.modList {
		if strings.EqualFold(m.folder, act) {
			return "Mod: " + m.caption
		}
	}
	return T("Originalwelt")
}

// pendingModName: Name der neuen Mod, die beim ersten Speichern angelegt wird.
func pendingModName() string {
	if activeModFolder() != "" {
		return ""
	}
	return strings.TrimSpace(loadConfig()["MDSNewModName"])
}

// askModName fragt nach einem Mod-Namen (Pflichtfeld, westeuropaeische
// Zeichen, weil der Mod Starter nur diese anzeigen kann).
func (a *app) askModName(title, prompt, def string) (string, bool) {
	for {
		name, ok := a.inputBox(title, prompt, def)
		name = strings.TrimSpace(name)
		if !ok {
			return "", false
		}
		if name == "" {
			messageBox(a.hwnd, T("Bitte einen Namen eingeben."), T(appTitleDE), MB_ICONWARNING)
			continue
		}
		if !cp1252OK(name) || strings.ContainsAny(name, "\r\n=[]") {
			messageBox(a.hwnd, T("Der Mod Starter kann nur lateinische Buchstaben (auch ä, ö, ü, ß), Ziffern und einfache Satzzeichen anzeigen. Bitte einen anderen Namen wählen."), T(appTitleDE), MB_ICONWARNING)
			def = name
			continue
		}
		return name, true
	}
}

// renameMod aendert den Anzeigenamen der bearbeiteten Mod (der Ordner bleibt).
func (a *app) renameMod() {
	act := activeModFolder()
	var m *modInfo
	for i := range a.modList {
		if strings.EqualFold(a.modList[i].folder, act) {
			m = &a.modList[i]
		}
	}
	if m == nil {
		return
	}
	name, ok := a.askModName(T("Mod umbenennen"), T("Neuer Name der Mod (so erscheint sie im Mod Starter):"), m.caption)
	if !ok || name == m.caption {
		return
	}
	old := m.caption
	writeModName(m.dir, name)
	a.renameInMDSList(old, name)
	a.modList = nil
	a.fillModList()
	a.updateTitle()
	a.flash(fmt.Sprintf(T("Mod umbenannt in „%s“. Ist sie im Mod Starter angehakt, bleibt sie es."), name))
}

// --- Kuerzel je Mod ----------------------------------------------------------
//
// Quests, Dialoge und davon abgeleitete Objekte bekommen interne Namen wie
// G3V_<Kuerzel>_Quest_01. So kommen sich zwei Mods, die gleichzeitig
// integriert sind, nicht in die Quere. Aeltere Mods ohne Kuerzel behalten
// G3V_ (sonst passten ihre vorhandenen Namen nicht mehr).

// readModCode liest das Kuerzel einer Mod aus ihrer Kennungsdatei.
func readModCode(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, modMarker))
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(l), "="); ok && strings.EqualFold(k, "Code") {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// writeModMarker schreibt die Kennungsdatei (mit Kuerzel, falls vorhanden).
func writeModMarker(dir, code string) {
	s := "AdanosEdit\r\n"
	if code != "" {
		s += "Code=" + code + "\r\n"
	}
	os.WriteFile(filepath.Join(dir, modMarker), []byte(s), 0644)
}

// uniqueModCode bildet aus einem Mod-Namen ein kurzes, freies Kuerzel.
func (a *app) uniqueModCode(name string) string {
	var b strings.Builder
	for _, r := range strings.TrimPrefix(modFolderName("", name), "AdanosEdit_") {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	base := b.String()
	if len(base) > 10 {
		base = base[:10]
	}
	if base == "" {
		base = "Mod"
	}
	used := map[string]bool{}
	for _, m := range a.viewerMods() {
		if c := readModCode(m.dir); c != "" {
			used[strings.ToLower(c)] = true
		}
	}
	code := base
	for i := 2; used[strings.ToLower(code)]; i++ {
		code = fmt.Sprintf("%s%d", base, i)
	}
	return code
}

// modCode liefert das Kuerzel der bearbeiteten Mod ("" = alte Mod ohne).
func (a *app) modCode() string {
	act := activeModFolder()
	if act == "" {
		return strings.TrimSpace(loadConfig()["MDSNewModCode"])
	}
	if mds := a.findMDS(); mds != "" {
		return readModCode(filepath.Join(mds, "FanMods", act))
	}
	return ""
}

// ownPrefix: Anfang der internen Namen neuer Quests und Dialoge.
func (a *app) ownPrefix() string {
	if c := a.modCode(); c != "" {
		return ownInfoPrefix + c + "_"
	}
	return ownInfoPrefix
}
