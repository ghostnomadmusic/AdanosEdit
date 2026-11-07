//go:build windows

package main

// Zwei Fassungen des Mod Starters sind verbreitet:
//
//   - MDS 1.2 (alt, vor dem Community Patch): Mods als *.p0x, integriert als
//     Data\<Name>.p00, .p01 ...
//   - MDS 1.2.0.1 "with CP 1.70 support" (aktuell, liegt meist in
//     Gothic 3\Data\MDS): Mods als *.m0x, integriert als Data\<Name>.m00 ...
//
// AdanosEdit erkennt die Fassung an der mds.exe und speichert passend.

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var mdsCPCache = map[string]bool{}

// mdsIsCP: true, wenn der Mod Starter in dir die CP-Fassung ist (*.m0x).
func mdsIsCP(dir string) bool {
	key := strings.ToLower(filepath.Clean(dir))
	if v, ok := mdsCPCache[key]; ok {
		return v
	}
	b, err := os.ReadFile(filepath.Join(dir, "mds.exe"))
	v := err == nil && bytes.Contains(b, []byte(`\*.m0x`)) && !bytes.Contains(b, []byte(`\*.p0x`))
	mdsCPCache[key] = v
	return v
}

// mdsLabel beschreibt einen Mod-Starter-Ordner fuer die Auswahl.
func mdsLabel(dir string) string {
	if mdsIsCP(dir) {
		return fmt.Sprintf(T("%s   (Mod Starter für den Community Patch, Mods als .m0x)"), dir)
	}
	return fmt.Sprintf(T("%s   (älterer Mod Starter, Mods als .p0x)"), dir)
}

// allMDS sucht alle Mod-Starter-Ordner (CP-Fassung zuerst).
func (a *app) allMDS() []string {
	var roots []string
	if a.gothicDir != "" {
		roots = append(roots, filepath.Join(a.gothicDir, "Data"), filepath.Join(a.gothicDir, "Tools"), a.gothicDir)
	}
	roots = append(roots, `C:\Gothic3Tools`, filepath.Dir(exeDir()), exeDir())
	var cands []string
	for _, root := range roots {
		cands = append(cands, root, filepath.Join(root, "mds"), filepath.Join(root, "MDS"))
		if ents, err := os.ReadDir(root); err == nil {
			for _, e := range ents {
				if e.IsDir() {
					cands = append(cands, filepath.Join(root, e.Name()))
				}
			}
		}
	}
	if d := loadConfig()["MDSDir"]; d != "" {
		cands = append([]string{d}, cands...)
	}
	seen := map[string]bool{}
	var cp, old []string
	for _, c := range cands {
		k := strings.ToLower(filepath.Clean(c))
		if seen[k] || !isMDSDir(c) {
			continue
		}
		seen[k] = true
		if mdsIsCP(c) {
			cp = append(cp, c)
		} else {
			old = append(old, c)
		}
	}
	return append(cp, old...)
}

// findMDS liefert den gewaehlten Mod-Starter-Ordner. Ohne Wahl wird der
// gefundene genommen (die CP-Fassung bevorzugt) und gemerkt.
func (a *app) findMDS() string {
	cfg := loadConfig()
	if isMDSDir(cfg["MDSDir"]) {
		return cfg["MDSDir"]
	}
	all := a.allMDS()
	if len(all) == 0 {
		return ""
	}
	cfg["MDSDir"] = all[0]
	saveConfig(cfg)
	return all[0]
}

// modExt ist die Dateiendung der Mod-Archive fuer den gewaehlten Mod Starter.
func (a *app) modExt() string {
	if m := a.findMDS(); m != "" && mdsIsCP(m) {
		return ".m0x"
	}
	return ".p0x"
}

// modArchive liefert den Pfad eines Archivs der Mod (z. B. "Infos").
func (a *app) modArchive(dir, base string) string {
	return filepath.Join(dir, base+a.modExt())
}

// syncModExt passt die Archive der eigenen Mods an den gewaehlten Mod
// Starter an (.p0x <-> .m0x). Der Inhalt bleibt gleich.
func (a *app) syncModExt() (renamed int) {
	mds := a.findMDS()
	if mds == "" {
		return 0
	}
	want := a.modExt()
	other := ".p0x"
	if want == ".p0x" {
		other = ".m0x"
	}
	ents, err := os.ReadDir(filepath.Join(mds, "FanMods"))
	if err != nil {
		return 0
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(mds, "FanMods", e.Name())
		if !fileExists(filepath.Join(dir, modMarker)) && !strings.EqualFold(e.Name(), mdsDefaultFolder) {
			continue
		}
		fs, _ := filepath.Glob(filepath.Join(dir, "*"+other))
		for _, f := range fs {
			to := strings.TrimSuffix(f, filepath.Ext(f)) + want
			if fileExists(to) {
				continue
			}
			if err := os.Rename(f, to); err != nil {
				log.Println("Umbenennen:", err)
				continue
			}
			log.Printf("Mod an Mod Starter angepasst: %s -> %s", filepath.Base(f), filepath.Base(to))
			renamed++
		}
	}
	return renamed
}

// chooseMDS laesst den Mod Starter waehlen (Neustart, wenn er sich aendert).
func (a *app) chooseMDS() {
	all := a.allMDS()
	cur := a.findMDS()
	var items []string
	sel := 0
	for i, d := range all {
		items = append(items, mdsLabel(d))
		if strings.EqualFold(d, cur) {
			sel = i
		}
	}
	items = append(items, T("Anderen Ordner auswählen …"))
	i, ok := a.listBox(T("Mod Starter wählen"), T("Mit welchem Mod Starter soll AdanosEdit arbeiten? Die eigenen Mods werden passend gespeichert (.m0x oder .p0x). Benutze im Spiel immer nur einen Mod Starter."), items, sel)
	if !ok {
		return
	}
	var d string
	if i < len(all) {
		d = all[i]
	} else {
		for {
			d = browseFolder(a.hwnd, T("Ordner des Mod Starters (mit mds.exe) auswählen"))
			if d == "" {
				return
			}
			if isMDSDir(d) {
				break
			}
			messageBox(a.hwnd, T("In diesem Ordner liegt keine mds.exe."), T(appTitleDE), MB_ICONERROR)
		}
	}
	if strings.EqualFold(filepath.Clean(d), filepath.Clean(cur)) {
		return
	}
	if a.ed.dirty {
		r := messageBoxRet(a.hwnd, T("Es gibt ungespeicherte Änderungen.\n\nVor dem Wechsel als Mod speichern?"), T(appTitleDE), MB_YESNOCANCEL|MB_ICONWARNING)
		if r == IDCANCEL {
			return
		}
		if r == IDYES {
			a.save()
			if a.ed.dirty {
				return
			}
		}
	}
	msg := fmt.Sprintf(T("Mod Starter wechseln zu\n%s?\n\nWichtig: Klicke vorher im bisherigen Mod Starter auf „Delete mods from game“, damit er seine Dateien aus dem Spiel entfernt.\n\nAdanosEdit startet danach neu."), d)
	if messageBoxRet(a.hwnd, msg, T(appTitleDE), MB_YESNO|MB_ICONQUESTION) != IDYES {
		return
	}
	// eigene Mods in den neuen Mod Starter uebernehmen
	keep := false
	os.MkdirAll(filepath.Join(d, "FanMods"), 0755)
	if mods := a.viewerMods(); len(mods) > 0 {
		if messageBoxRet(a.hwnd, fmt.Sprintf(T("Deine %d AdanosEdit-Mod(s) in den neuen Mod Starter kopieren?\n(Die Dateiendungen werden dabei passend geändert.)"), len(mods)), T(appTitleDE), MB_YESNO|MB_ICONQUESTION) == IDYES {
			for _, m := range mods {
				to := filepath.Join(d, "FanMods", m.folder)
				if fileExists(to) {
					continue
				}
				if err := copyDir(m.dir, to); err != nil {
					messageBox(a.hwnd, fmt.Sprintf(T("Der Mod-Ordner konnte nicht angelegt werden:\n%s"), err.Error()), T(appTitleDE), MB_ICONERROR)
					return
				}
			}
			keep = true
		}
	}
	cfg := loadConfig()
	cfg["MDSDir"] = d
	cfg["SaveMode"] = "mds"
	if f := activeModFolder(); f != "" && fileExists(filepath.Join(d, "FanMods", f)) {
		keep = true
	}
	if !keep {
		cfg["MDSModFolder"] = newModValue
	}
	cfg["ViewDist"] = fmt.Sprintf("%.0f", a.viewDist)
	cfg["CamPos"] = fmt.Sprintf("%.0f %.0f %.0f %.4f %.4f", a.cam.Pos[0], a.cam.Pos[1], a.cam.Pos[2], a.cam.Yaw, a.cam.Pitch)
	saveConfig(cfg)
	a.restart()
}

// copyDir kopiert einen Ordner (ohne Unterordner-Rekursion ueber Links).
func copyDir(from, to string) error {
	return filepath.WalkDir(from, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		dst := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0644)
	})
}
