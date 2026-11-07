package g3

import (
	"bytes"
	"encoding/binary"
	"os"
	"sort"
	"strings"
	"time"
)

// PakFile ist eine Datei, die in ein neues Archiv geschrieben wird.
type PakFile struct {
	Name string // Pfad im Archiv mit "/"
	Data []byte
}

type pakDir struct {
	name  string
	dirs  map[string]*pakDir
	files []*PakFile
}

func filetime(t time.Time) uint64 {
	return uint64(t.UnixNano()/100) + 116444736000000000
}

// WritePak schreibt ein unkomprimiertes G3V0-Archiv (wie .pak/.mod).
func WritePak(path string, files []PakFile) error {
	root := &pakDir{dirs: map[string]*pakDir{}}
	for i := range files {
		f := &files[i]
		f.Name = strings.Trim(strings.ReplaceAll(f.Name, "\\", "/"), "/")
		parts := strings.Split(f.Name, "/")
		d := root
		for k := 0; k < len(parts)-1; k++ {
			key := strings.ToLower(parts[k])
			sub := d.dirs[key]
			if sub == nil {
				sub = &pakDir{name: strings.Join(parts[:k+1], "/") + "/", dirs: map[string]*pakDir{}}
				d.dirs[key] = sub
			}
			d = sub
		}
		d.files = append(d.files, f)
	}
	var buf bytes.Buffer
	buf.Write(make([]byte, 48))
	offsets := map[*PakFile]uint64{}
	// Dateidaten in Verzeichnisreihenfolge
	var writeData func(d *pakDir)
	writeData = func(d *pakDir) {
		for _, k := range sortedKeys(d.dirs) {
			writeData(d.dirs[k])
		}
		sort.Slice(d.files, func(a, b int) bool { return strings.ToLower(d.files[a].Name) < strings.ToLower(d.files[b].Name) })
		for _, f := range d.files {
			offsets[f] = uint64(buf.Len())
			buf.Write(f.Data)
		}
	}
	writeData(root)
	dirOff := uint64(buf.Len())
	now := filetime(time.Now())
	le := binary.LittleEndian
	u32 := func(v uint32) { binary.Write(&buf, le, v) }
	u64 := func(v uint64) { binary.Write(&buf, le, v) }
	str := func(s string) {
		u32(uint32(len(s)))
		if len(s) > 0 {
			buf.WriteString(s)
			buf.WriteByte(0)
		}
	}
	var writeDir func(d *pakDir)
	writeDir = func(d *pakDir) {
		u64(now)
		u64(now)
		u64(now)
		u64(0)
		u32(0x00002010)
		str(d.name)
		keys := sortedKeys(d.dirs)
		u32(uint32(len(keys)))
		for _, k := range keys {
			writeDir(d.dirs[k])
		}
		u32(uint32(len(d.files)))
		for _, f := range d.files {
			u64(now)
			u64(now)
			u64(now)
			u64(0)
			u32(0x00002020)
			u64(offsets[f])
			u64(uint64(len(f.Data)))
			u64(uint64(len(f.Data)))
			u32(0)
			u32(0)
			str(f.Name)
			str(f.Name)
		}
	}
	writeDir(root)
	end := uint64(buf.Len())
	u32(0)
	b := buf.Bytes()
	copy(b[4:8], "G3V0")
	le.PutUint32(b[16:], 1)
	le.PutUint32(b[20:], 0x00323C3F)
	le.PutUint64(b[24:], dirOff)
	le.PutUint64(b[32:], dirOff)
	le.PutUint64(b[40:], end)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return replaceFile(tmp, path, b)
}

// replaceFile ersetzt path durch tmp. Unter Windows schlaegt das Umbenennen
// fehl, solange ein anderes Programm (z. B. ein zweites Viewer-Fenster, der
// Virenscanner oder die Explorer-Vorschau) die Datei offen hat. Dann wird
// kurz gewartet und notfalls der Inhalt direkt in die bestehende Datei
// geschrieben.
func replaceFile(tmp, path string, b []byte) error {
	var err error
	for i := 0; i < 6; i++ {
		if err = os.Rename(tmp, path); err == nil {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	f, err2 := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	if err2 != nil {
		return err
	}
	if _, err2 = f.Write(b); err2 != nil {
		f.Close()
		return err2
	}
	if err2 = f.Close(); err2 != nil {
		return err2
	}
	os.Remove(tmp)
	return nil
}

func sortedKeys(m map[string]*pakDir) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
