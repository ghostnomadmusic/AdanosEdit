//go:build windows

package main

// Anbindung an den Gothic 3 Mod Starter (PiranhaClub & Mighty DWARF).
//
// Der MDS erwartet jede Mod als eigenen Unterordner in <MDS>\FanMods mit
// *.m0x-Dateien (CP-Fassung) bzw. *.p0x (alte Fassung, siehe mdsvariant.go);
// sie werden beim Integrieren als naechste Generation (z. B.
// Projects_compiled.m00) nach Gothic3\Data kopiert, genau einem *.jpg
// (Vorschaubild 330x327), genau einer *.rtf (Beschreibung), einer readme.txt
// und optional ModName.ini mit [Name] Caption=...
// Welche Dateien er kopiert hat, merkt er sich in fdm.dat; beim Abwaehlen und
// erneuten Integrieren entfernt er sie wieder.

import (
	"bytes"
	"fmt"
	"g3viewer/g3"
	"g3viewer/lang"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unsafe"
)

const (
	mdsDefaultFolder  = "G3Viewer_Weltaenderungen"
	mdsDefaultCaption = "AdanosEdit – eigene Weltänderungen"
)

func isMDSDir(d string) bool {
	if d == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(d, "mds.exe"))
	return err == nil
}

// mdsModDir liefert den Ordner der bearbeiteten Mod.
func (a *app) mdsModDir(mds string) string {
	f := activeModFolder()
	if f == "" {
		return "" // neue Mod, Name wird beim Speichern abgefragt
	}
	return filepath.Join(mds, "FanMods", f)
}

// saveMode liefert "mds" oder "data"; beim ersten Speichern wird gefragt.
// Leerer Rueckgabewert = abgebrochen.
func (a *app) saveMode() (mode, mds string) {
	cfg := loadConfig()
	switch cfg["SaveMode"] {
	case "data":
		return "data", ""
	case "mds":
		if m := a.findMDS(); m != "" {
			return "mds", m
		}
	case "":
		// noch nicht gewaehlt, aber ein Mod Starter mit eigener Mod ist da
		if m := a.findMDS(); m != "" && len(a.viewerMods()) > 0 {
			cfg["SaveMode"] = "mds"
			saveConfig(cfg)
			return "mds", m
		}
	}
	m := a.findMDS()
	text := T("Wie sollen die Änderungen gespeichert werden?\n\nJa  =  als Mod für den Gothic 3 Mod Starter (empfohlen).\n         Sie erscheint dort in der Liste und lässt sich an- und abwählen.\n\nNein  =  direkt als Projects_compiled.mod in den Data-Ordner\n         (immer aktiv, bis du die Datei löschst oder umbenennst).\n")
	if m != "" {
		text += fmt.Sprintf(T("\nGefundener Mod Starter: %s"), m)
	} else {
		text += T("\nDer Mod Starter wurde nicht gefunden – bei „Ja“ kannst du seinen Ordner auswählen.")
	}
	text += T("\n\nDie Wahl lässt sich später in AdanosEdit.ini (SaveMode=mds oder data) ändern.")
	switch messageBoxRet(a.hwnd, text, T(appTitleDE), MB_YESNOCANCEL|MB_ICONINFORMATION) {
	case IDYES:
		for m == "" {
			d := browseFolder(a.hwnd, T("Ordner des Mod Starters (mit mds.exe) auswählen"))
			if d == "" {
				return "", ""
			}
			if isMDSDir(d) {
				m = d
				break
			}
			messageBox(a.hwnd, T("In diesem Ordner liegt keine mds.exe."), T(appTitleDE), MB_ICONERROR)
		}
		cfg = loadConfig()
		cfg["SaveMode"] = "mds"
		cfg["MDSDir"] = m
		saveConfig(cfg)
		return "mds", m
	case IDNO:
		cfg = loadConfig()
		cfg["SaveMode"] = "data"
		saveConfig(cfg)
		return "data", ""
	}
	return "", ""
}

// mdsPakFiles liefert alle Archive (*.p0x bzw. *.m0x) der bearbeiteten Mod.
func (a *app) mdsPakFiles() []string {
	p := a.mdsPakFile()
	if p == "" {
		return nil
	}
	m, _ := filepath.Glob(filepath.Join(filepath.Dir(p), "*"+a.modExt()))
	return m
}

// mdsPakFile liefert die p0x-Datei der eigenen Mod, falls der MDS-Modus aktiv ist.
func (a *app) mdsPakFile() string {
	// Ohne Einstellung (z. B. neue Version in einem frischen Ordner) gilt der
	// Mod Starter, sobald einer gefunden wird - sonst fehlte die eigene Mod.
	cfg := loadConfig()
	if cfg["SaveMode"] == "data" {
		return ""
	}
	m := a.findMDS()
	if m == "" {
		return ""
	}
	d := a.mdsModDir(m)
	if d == "" {
		return ""
	}
	return a.modArchive(d, "Projects_compiled")
}

// integratedCopy sucht im Data-Ordner die vom MDS dorthin kopierte Fassung
// der eigenen Mod (gleicher Inhalt wie die bisherige p0x-Datei).
func (a *app) integratedCopy(old []byte) string { return a.integratedCopyOf(old, "Projects_compiled") }

// integratedCopyOf sucht die integrierte Kopie eines Archivs (Basisname z. B. Templates).
func (a *app) integratedCopyOf(old []byte, base string) string {
	if len(old) == 0 {
		return ""
	}
	data := filepath.Join(a.gothicDir, "Data")
	ents, err := os.ReadDir(data)
	if err != nil {
		return ""
	}
	re := regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(base) + `\.[pm]\d\d$`)
	for _, e := range ents {
		if e.IsDir() || !re.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil || info.Size() != int64(len(old)) {
			continue
		}
		p := filepath.Join(data, e.Name())
		if b, err := os.ReadFile(p); err == nil && bytes.Equal(b, old) {
			return p
		}
	}
	return ""
}

// writeMDSExtras legt Vorschaubild, Beschreibung und Namen der Mod an.
func (a *app) writeMDSExtras(dir string, nWorld int) {
	cfg := loadConfig()
	caption := modCaption(dir)
	oldCaption := caption
	if strings.HasPrefix(caption, "G3 Viewer – ") || strings.HasPrefix(caption, "G3 Viewer - ") {
		caption = "AdanosEdit" + strings.TrimPrefix(strings.TrimPrefix(caption, "G3 Viewer"), "")
	}
	if caption == "" {
		caption = cfg["MDSModCaption"]
	}
	if caption == "" {
		caption = T(mdsDefaultCaption)
		if !cp1252OK(caption) {
			caption = lang.English(mdsDefaultCaption)
		}
	}
	// Vorschaubild: aktuelles 3D-Bild (bei jedem Speichern neu)
	if a.lastShot != nil {
		var buf bytes.Buffer
		if jpeg.Encode(&buf, a.lastShot, &jpeg.Options{Quality: 88}) == nil {
			// Nur eine einzige *.jpg darf im Ordner liegen
			if ents, err := os.ReadDir(dir); err == nil {
				for _, e := range ents {
					if strings.EqualFold(filepath.Ext(e.Name()), ".jpg") && !strings.EqualFold(e.Name(), "Vorschau.jpg") {
						os.Remove(filepath.Join(dir, e.Name()))
					}
				}
			}
			os.WriteFile(filepath.Join(dir, "Vorschau.jpg"), buf.Bytes(), 0644)
		}
	}
	stamp := time.Now().Format("02.01.2006 15:04")
	desc := fmt.Sprintf(T("Mit AdanosEdit bearbeitete Welt (verschobene, kopierte, neue und gelöschte Objekte).\r\nZuletzt gespeichert: %s\r\n%d Weltdatei(en) geändert.\r\nÄndert nur Projects_compiled. Wirkt in einem neuen Spiel."), stamp, nWorld)
	if _, err := os.Stat(filepath.Join(dir, "readme.txt")); err != nil || cfg["MDSKeepTexts"] != "1" {
		readme := toCP1252(desc)
		if !cp1252OK(desc) {
			readme = append([]byte{0xEF, 0xBB, 0xBF}, desc...)
		}
		os.WriteFile(filepath.Join(dir, "readme.txt"), readme, 0644)
		os.WriteFile(filepath.Join(dir, "Beschreibung.rtf"), []byte(rtf(caption, desc)), 0644)
	}
	writeModName(dir, caption)
	if oldCaption != "" && oldCaption != caption {
		a.renameInMDSList(oldCaption, caption)
	}
	writeModMarker(dir, readModCode(dir))
}

// writeModName schreibt den Anzeigenamen fuer den Mod Starter.
func writeModName(dir, caption string) {
	if !cp1252OK(caption) {
		caption = filepath.Base(dir)
	}
	ini := "[Name]\r\nCaption=" + caption + "\r\n"
	os.WriteFile(filepath.Join(dir, "ModName.ini"), toCP1252(ini), 0644)
}

func toCP1252(s string) []byte {
	var out []byte
	for _, r := range s {
		switch {
		case r < 0x80:
			out = append(out, byte(r))
		case r >= 0xA0 && r <= 0xFF:
			out = append(out, byte(r))
		case r == '–':
			out = append(out, 0x96)
		default:
			out = append(out, '?')
		}
	}
	return out
}

func rtf(title, body string) string {
	esc := func(s string) string {
		var b strings.Builder
		for _, r := range s {
			switch {
			case r == '\\' || r == '{' || r == '}':
				b.WriteByte('\\')
				b.WriteRune(r)
			case r == '\r':
			case r == '\n':
				b.WriteString("\\par\r\n")
			case r < 0x80:
				b.WriteRune(r)
			default:
				fmt.Fprintf(&b, "\\u%d?", int16(r))
			}
		}
		return b.String()
	}
	return "{\\rtf1\\ansi\\ansicpg1252\\deff0{\\fonttbl{\\f0 Arial;}}\\f0\\fs20{\\b " + esc(title) + "}\\par\\par\r\n" + esc(body) + "\\par}\r\n"
}

// grabShot liest das aktuelle 3D-Bild und schneidet ein 330x327-Vorschaubild zu.
func (a *app) grabShot() {
	w, h := a.width, a.height
	if w < 16 || h < 16 {
		return
	}
	px := make([]byte, w*h*4)
	pglReadPixels.Call(0, 0, uintptr(w), uintptr(h), GL_RGBA, GL_UNSIGNED_BYTE, uintptr(unsafe.Pointer(&px[0])))
	const tw, th = 330, 327
	// groesster Ausschnitt mit passendem Seitenverhaeltnis, mittig
	cw, ch := w, w*th/tw
	if ch > h {
		ch, cw = h, h*tw/th
	}
	x0, y0 := (w-cw)/2, (h-ch)/2
	img := image.NewRGBA(image.Rect(0, 0, tw, th))
	for y := 0; y < th; y++ {
		for x := 0; x < tw; x++ {
			// Box-Filter ueber die Quellpixel
			sx0, sx1 := x0+x*cw/tw, x0+(x+1)*cw/tw
			sy0, sy1 := y0+y*ch/th, y0+(y+1)*ch/th
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			if sy1 <= sy0 {
				sy1 = sy0 + 1
			}
			var r, g, b, n int
			for sy := sy0; sy < sy1; sy++ {
				row := (h - 1 - sy) * w * 4
				for sx := sx0; sx < sx1; sx++ {
					o := row + sx*4
					r += int(px[o])
					g += int(px[o+1])
					b += int(px[o+2])
					n++
				}
			}
			img.Set(x, y, color.RGBA{uint8(r / n), uint8(g / n), uint8(b / n), 255})
		}
	}
	a.lastShot = img
}

// cp1252OK prueft, ob ein Text ohne Verlust in Windows-1252 passt.
func cp1252OK(s string) bool {
	for _, r := range s {
		if r >= 0x80 && !(r >= 0xA0 && r <= 0xFF) && r != '–' {
			return false
		}
	}
	return true
}

// renameInMDSList passt die Liste der angehakten Mods des Mod Starters
// (flm.dat) an, wenn sich der Anzeigename einer Mod aendert. Sonst findet der
// Mod Starter die Mod nicht mehr ("List index out of bounds (-1)").
func (a *app) renameInMDSList(oldCap, newCap string) {
	mds := a.findMDS()
	if mds == "" {
		return
	}
	p := filepath.Join(mds, "flm.dat")
	b, err := os.ReadFile(p)
	if err != nil {
		return
	}
	enc := func(s string) string {
		if cp1252OK(s) {
			return string(toCP1252(s))
		}
		return s
	}
	lines := strings.Split(string(b), "\r\n")
	changed := false
	for i, l := range lines {
		if l == enc(oldCap) || l == oldCap {
			lines[i] = enc(newCap)
			changed = true
		}
	}
	if changed {
		os.WriteFile(p, []byte(strings.Join(lines, "\r\n")), 0644)
	}
}

// modStrings liest die Texte der bearbeiteten Mod (stringtableMod.ini).
func (a *app) modStrings() map[string]string {
	mds := a.findMDS()
	if mds == "" || loadConfig()["SaveMode"] == "data" {
		return nil
	}
	dir := a.mdsModDir(mds)
	if dir == "" {
		return nil
	}
	d, err := os.ReadFile(filepath.Join(dir, "stringtableMod.ini"))
	if err != nil {
		return nil
	}
	return g3.StringValues(d)
}
