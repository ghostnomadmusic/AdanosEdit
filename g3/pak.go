// Package g3 liest Gothic-3-Datenformate (Pak-Archive, Welt-Sektoren,
// Meshes, Materialien, Texturen).
package g3

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// PakEntry ist eine Datei innerhalb eines Pak-Archivs.
type PakEntry struct {
	Name     string // Pfad innerhalb des Archivs, mit "/" getrennt
	Offset   uint64
	Stored   uint64 // Groesse im Archiv
	Size     uint64 // entpackte Groesse
	Compress uint32 // 0 = keine, 2 = zlib
	Archive  *Pak
}

// Pak ist ein geoeffnetes Gothic-3-Archiv (G3V0).
type Pak struct {
	Path    string
	f       *os.File
	Entries []*PakEntry
}

type pakReader struct {
	d   []byte
	pos int
	err error
}

func (r *pakReader) need(n int) bool {
	if r.err != nil {
		return false
	}
	if r.pos+n > len(r.d) {
		r.err = fmt.Errorf("Verzeichnis endet unerwartet bei %d", r.pos)
		return false
	}
	return true
}
func (r *pakReader) u32() uint32 {
	if !r.need(4) {
		return 0
	}
	v := binary.LittleEndian.Uint32(r.d[r.pos:])
	r.pos += 4
	return v
}
func (r *pakReader) u64() uint64 {
	if !r.need(8) {
		return 0
	}
	v := binary.LittleEndian.Uint64(r.d[r.pos:])
	r.pos += 8
	return v
}
func (r *pakReader) str() string {
	n := int(r.u32())
	if n == 0 || r.err != nil {
		return ""
	}
	if !r.need(n + 1) {
		return ""
	}
	s := string(r.d[r.pos : r.pos+n])
	r.pos += n + 1
	return s
}

// OpenPak oeffnet ein Archiv und liest sein Inhaltsverzeichnis.
func OpenPak(path string) (*Pak, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	hdr := make([]byte, 48)
	if _, err := f.ReadAt(hdr, 0); err != nil {
		f.Close()
		return nil, fmt.Errorf("%s: Kopf nicht lesbar: %w", path, err)
	}
	if string(hdr[4:8]) != "G3V0" {
		f.Close()
		return nil, fmt.Errorf("%s: kein Gothic-3-Archiv", path)
	}
	dirOff := binary.LittleEndian.Uint64(hdr[32:])
	dirEnd := binary.LittleEndian.Uint64(hdr[40:])
	st, _ := f.Stat()
	if dirEnd < dirOff || int64(dirEnd) > st.Size() {
		dirEnd = uint64(st.Size())
	}
	dir := make([]byte, dirEnd-dirOff)
	if _, err := f.ReadAt(dir, int64(dirOff)); err != nil && err != io.EOF {
		f.Close()
		return nil, fmt.Errorf("%s: Verzeichnis nicht lesbar: %w", path, err)
	}
	p := &Pak{Path: path, f: f}
	r := &pakReader{d: dir}
	p.readDir(r, 0)
	if r.err != nil {
		f.Close()
		return nil, fmt.Errorf("%s: %w", path, r.err)
	}
	return p, nil
}

func (p *Pak) readDir(r *pakReader, depth int) {
	if depth > 64 {
		r.err = fmt.Errorf("Verzeichnis zu tief verschachtelt")
		return
	}
	r.pos += 24 // Zeitstempel
	r.u64()
	attr := r.u32()
	if attr&0x10 == 0 && r.err == nil {
		r.err = fmt.Errorf("Ordner erwartet bei %d", r.pos)
		return
	}
	r.str()
	nd := int(r.u32())
	for i := 0; i < nd && r.err == nil; i++ {
		p.readDir(r, depth+1)
	}
	nf := int(r.u32())
	for i := 0; i < nf && r.err == nil; i++ {
		r.pos += 24
		r.u64()
		r.u32()
		e := &PakEntry{Archive: p}
		e.Offset = r.u64()
		e.Stored = r.u64()
		e.Size = r.u64()
		r.u32() // Verschluesselung (immer 0)
		e.Compress = r.u32()
		e.Name = strings.ReplaceAll(r.str(), "\\", "/")
		r.str() // urspruenglicher Pfad beim Packen
		p.Entries = append(p.Entries, e)
	}
}

// Read liest und entpackt eine Datei.
func (e *PakEntry) Read() ([]byte, error) {
	buf := make([]byte, e.Stored)
	if _, err := e.Archive.f.ReadAt(buf, int64(e.Offset)); err != nil && err != io.EOF {
		return nil, err
	}
	switch e.Compress {
	case 0:
		return buf, nil
	case 2:
		zr, err := zlib.NewReader(bytes.NewReader(buf))
		if err != nil {
			return nil, err
		}
		out := make([]byte, 0, e.Size)
		w := bytes.NewBuffer(out)
		if _, err := io.Copy(w, zr); err != nil {
			return nil, err
		}
		return w.Bytes(), nil
	}
	return nil, fmt.Errorf("unbekannte Kompression %d", e.Compress)
}

// Close schliesst das Archiv.
func (p *Pak) Close() { p.f.Close() }

// VFS fasst alle Archive des Data-Ordners zusammen. Spaetere Generationen
// (.p00, .p01 ..., .mod, .m00 ...) ueberschreiben fruehere.
type VFS struct {
	DataDir  string
	Archives []*Pak
	byPath   map[string]*PakEntry            // "projects_compiled/g3_world_01/..." -> Eintrag
	byBase   map[string]map[string]*PakEntry // kategorie -> dateiname -> Eintrag
	mu       sync.Mutex
	Warnings []string
}

// archiveGenerations liefert die Dateien eines Archivs in Ladereihenfolge.
func archiveGenerations(dataDir, base string, exists func(string) bool) []string {
	var out []string
	add := func(ext0 string, prefix byte) {
		first := filepath.Join(dataDir, base+"."+ext0)
		if !exists(first) {
			return
		}
		out = append(out, first)
		for i := 0; i < 100; i++ {
			p := filepath.Join(dataDir, fmt.Sprintf("%s.%c%02d", base, prefix, i))
			if !exists(p) {
				break
			}
			out = append(out, p)
		}
	}
	add("pak", 'p')
	// Community Patch: liegt zwischen den Spielarchiven und den Mods
	if cpt := filepath.Join(dataDir, base+".cpt"); exists(cpt) {
		out = append(out, cpt)
	}
	add("mod", 'm')
	return out
}

// OpenVFS durchsucht den Data-Ordner und oeffnet alle relevanten Archive.
func OpenVFS(dataDir string, onlyBases ...string) (*VFS, error) {
	ents, err := os.ReadDir(dataDir)
	if err != nil {
		return nil, err
	}
	present := map[string]string{}
	bases := map[string]string{}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		present[strings.ToLower(n)] = n
		ext := strings.ToLower(filepath.Ext(n))
		if ext == ".pak" || ext == ".mod" {
			b := n[:len(n)-len(ext)]
			bases[strings.ToLower(b)] = b
		}
	}
	exists := func(p string) bool {
		_, ok := present[strings.ToLower(filepath.Base(p))]
		return ok
	}
	v := &VFS{DataDir: dataDir, byPath: map[string]*PakEntry{}, byBase: map[string]map[string]*PakEntry{}}
	var keys []string
	for k := range bases {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, lb := range keys {
		if len(onlyBases) > 0 {
			ok := false
			for _, o := range onlyBases {
				if strings.EqualFold(o, lb) {
					ok = true
				}
			}
			if !ok {
				continue
			}
		}
		b := bases[lb]
		for _, file := range archiveGenerations(dataDir, b, exists) {
			real := filepath.Join(dataDir, present[strings.ToLower(filepath.Base(file))])
			p, err := OpenPak(real)
			if err != nil {
				v.Warnings = append(v.Warnings, err.Error())
				continue
			}
			v.Archives = append(v.Archives, p)
			cat := strings.ToLower(b)
			if v.byBase[cat] == nil {
				v.byBase[cat] = map[string]*PakEntry{}
			}
			for _, e := range p.Entries {
				if e.Size == 0 {
					// Leere Dateien in Patch-Archiven: aeltere Version behalten.
					continue
				}
				v.byPath[cat+"/"+strings.ToLower(e.Name)] = e
				base := strings.ToLower(e.Name)
				if i := strings.LastIndex(base, "/"); i >= 0 {
					base = base[i+1:]
				}
				v.byBase[cat][base] = e
			}
		}
	}
	if len(v.Archives) == 0 {
		return nil, fmt.Errorf("keine Archive in %s gefunden", dataDir)
	}
	return v, nil
}

// Find sucht eine Datei per Archivname und Pfad, z. B. ("Projects_compiled", "G3_World_01/...").
func (v *VFS) Find(category, path string) *PakEntry {
	return v.byPath[strings.ToLower(category)+"/"+strings.ToLower(strings.ReplaceAll(path, "\\", "/"))]
}

// FindBase sucht eine Datei nur ueber den Dateinamen, z. B. ("_compiledMesh", "G3_Object_Torch_01.xcmsh").
func (v *VFS) FindBase(category, name string) *PakEntry {
	m := v.byBase[strings.ToLower(category)]
	if m == nil {
		return nil
	}
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return m[strings.ToLower(name)]
}

// List liefert alle Pfade einer Kategorie (sortiert, Originalschreibweise).
func (v *VFS) List(category string) []*PakEntry {
	prefix := strings.ToLower(category) + "/"
	var out []*PakEntry
	for k, e := range v.byPath {
		if strings.HasPrefix(k, prefix) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// Close schliesst alle Archive.
func (v *VFS) Close() {
	for _, a := range v.Archives {
		a.Close()
	}
}

// Reopen ersetzt ein (geschlossenes) Archiv durch die neu geschriebene Datei
// bzw. fuegt sie hinzu und aktualisiert die Verzeichnisse.
func (v *VFS) Reopen(path string, old *Pak) error {
	p, err := OpenPak(path)
	if err != nil {
		return err
	}
	cat := strings.ToLower(filepath.Base(path))
	cat = cat[:len(cat)-len(filepath.Ext(cat))]
	found := false
	for i, a := range v.Archives {
		if a == old && old != nil {
			v.Archives[i] = p
			found = true
		}
	}
	if !found {
		v.Archives = append(v.Archives, p)
	}
	if v.byBase[cat] == nil {
		v.byBase[cat] = map[string]*PakEntry{}
	}
	// Eintraege des alten Archivs entfernen, neue eintragen
	if old != nil {
		for k, e := range v.byPath {
			if e.Archive == old {
				delete(v.byPath, k)
			}
		}
		for k, e := range v.byBase[cat] {
			if e.Archive == old {
				delete(v.byBase[cat], k)
			}
		}
	}
	for _, e := range p.Entries {
		if e.Size == 0 {
			continue
		}
		v.byPath[cat+"/"+strings.ToLower(e.Name)] = e
		base := strings.ToLower(e.Name)
		if i := strings.LastIndex(base, "/"); i >= 0 {
			base = base[i+1:]
		}
		v.byBase[cat][base] = e
	}
	return nil
}

// Drop entfernt ein Archiv aus dem VFS, schliesst es und baut die
// Verzeichnisse neu auf (darunterliegende Generationen werden wieder sichtbar).
func (v *VFS) Drop(old *Pak) {
	for i, a := range v.Archives {
		if a == old {
			v.Archives = append(v.Archives[:i], v.Archives[i+1:]...)
			break
		}
	}
	old.Close()
	v.byPath = map[string]*PakEntry{}
	v.byBase = map[string]map[string]*PakEntry{}
	for _, p := range v.Archives {
		cat := strings.ToLower(filepath.Base(p.Path))
		cat = cat[:len(cat)-len(filepath.Ext(cat))]
		if v.byBase[cat] == nil {
			v.byBase[cat] = map[string]*PakEntry{}
		}
		for _, e := range p.Entries {
			if e.Size == 0 {
				continue
			}
			v.byPath[cat+"/"+strings.ToLower(e.Name)] = e
			base := strings.ToLower(e.Name)
			if j := strings.LastIndex(base, "/"); j >= 0 {
				base = base[j+1:]
			}
			v.byBase[cat][base] = e
		}
	}
}
