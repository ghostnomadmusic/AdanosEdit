package g3

// Navigationskarte (G3_World_01/NavigationMap.xnav).
//
// Gothic 3 erkennt Zonen (z. B. fuer "Ort erreichen") nur, wenn sie in dieser
// vorberechneten Karte stehen. Der Aufbau folgt g3dit von georgeto
// (de.george.navmap, Apache License 2.0). Geaendert wird nur durch Einfuegen
// an den passenden Stellen (wie g3dit beim Eintragen einer Zone):
//
//   - Section1 (Raster): in jede Rasterzelle, die der Zonenkreis beruehrt,
//     einen Verweis auf die Zone (vor den Wegen der Zelle),
//   - Section3b2 (Zonenliste): Zone mit freier Gruppennummer (Cluster),
//   - Section3fg (Objektliste): Zone ohne Verbindungen.
//
// Alles andere bleibt Byte fuer Byte erhalten.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// NavZoneInfo beschreibt eine einzutragende Zone.
type NavZoneInfo struct {
	GUID   [16]byte
	Center [3]float32 // Mittelpunkt des Radius (Welt)
	Radius float32
}

type navCellEntry struct {
	off    int  // Beginn des Verweises
	isPath bool // gCNavPath_PS
}

type navCell struct {
	countOff int // Position der Anzahl (int32)
	end      int // Ende der Liste
	entries  []navCellEntry
}

// NavMap ist eine eingelesene Navigationskarte.
type NavMap struct {
	data                   []byte
	nx, nz                 int
	cells                  [][]navCell
	minX, maxX, minZ, maxZ float32
	cellX, cellZ           float32
	zoneProxyName          []byte // Stringtabellen-Verweis "gCNavZone_PS" (2 Byte)
	sec3b2Count, sec3b2End int
	sec3fgCount, sec3fgEnd int
	clusters               map[int32]bool
	zones                  map[[16]byte]bool
}

var errNavFormat = errors.New("NavigationMap: unbekanntes Format")

type navReader struct {
	d   []byte
	pos int
	err error
}

func (r *navReader) need(n int) bool {
	if r.err != nil {
		return false
	}
	if r.pos+n > len(r.d) {
		r.err = errNavFormat
		return false
	}
	return true
}
func (r *navReader) skip(n int) {
	if r.need(n) {
		r.pos += n
	}
}
func (r *navReader) u8() byte {
	if !r.need(1) {
		return 0
	}
	r.pos++
	return r.d[r.pos-1]
}
func (r *navReader) u16() int {
	if !r.need(2) {
		return 0
	}
	r.pos += 2
	return int(binary.LittleEndian.Uint16(r.d[r.pos-2:]))
}
func (r *navReader) i32() int32 {
	if !r.need(4) {
		return 0
	}
	r.pos += 4
	return int32(binary.LittleEndian.Uint32(r.d[r.pos-4:]))
}
func (r *navReader) f32() float32 { return math.Float32frombits(uint32(r.i32())) }

// list liest Listenkopf (1 Byte + int32) und liefert Anzahl und Position der Anzahl.
func (r *navReader) list() (int, int) {
	r.skip(1)
	off := r.pos
	n := r.i32()
	if n < 0 || n > 10000000 {
		r.err = errNavFormat
		return 0, off
	}
	return int(n), off
}

// proxy liest eCPropertySetProxy: Name (oder leer) und GUID.
func (r *navReader) proxy() (name []byte, guid []byte) {
	r.skip(2)
	if r.u8() == 1 {
		if r.need(2) {
			name = r.d[r.pos : r.pos+2]
		}
		r.skip(2)
	}
	r.skip(2)
	if r.u8() == 1 {
		if r.need(20) {
			guid = r.d[r.pos : r.pos+20]
		}
		r.skip(20)
	}
	return
}

func (r *navReader) vecList() {
	n, _ := r.list()
	r.skip(12 * n)
}

// ParseNavMap liest eine NavigationMap.xnav.
func ParseNavMap(d []byte) (*NavMap, error) {
	if len(d) < 40 || string(d[:8]) != "GENOMFLE" || string(d[14:25]) != "GE3-NAV-MAP" {
		return nil, errNavFormat
	}
	r := &navReader{d: d, pos: 25}
	if r.i32() != 3 || r.i32() != 0 {
		return nil, fmt.Errorf("NavigationMap: Version wird nicht unterstuetzt")
	}
	m := &NavMap{data: d, clusters: map[int32]bool{}, zones: map[[16]byte]bool{}}
	// Section1: Raster
	nx, _ := r.list()
	m.nx = nx
	m.cells = make([][]navCell, nx)
	for x := 0; x < nx && r.err == nil; x++ {
		nz, _ := r.list()
		if x == 0 {
			m.nz = nz
		} else if nz != m.nz {
			return nil, errNavFormat
		}
		m.cells[x] = make([]navCell, nz)
		for z := 0; z < nz && r.err == nil; z++ {
			n, off := r.list()
			c := navCell{countOff: off}
			for k := 0; k < n && r.err == nil; k++ {
				e := navCellEntry{off: r.pos}
				r.proxy()
				c.entries = append(c.entries, e)
			}
			c.end = r.pos
			m.cells[x][z] = c
		}
	}
	m.minX, m.maxX, m.minZ, m.maxZ = r.f32(), r.f32(), r.f32(), r.f32()
	m.cellX, m.cellZ = r.f32(), r.f32()
	// Section2: NegZones
	n, _ := r.list()
	for k := 0; k < n && r.err == nil; k++ {
		r.vecList()
		r.skip(4 + 12 + 1 + 20 + 20)
	}
	// Section3a: NegCircles
	n, _ = r.list()
	for k := 0; k < n && r.err == nil; k++ {
		r.vecList()
		c, _ := r.list()
		r.skip(4 * c)
		r.skip(4)
		c, _ = r.list()
		r.skip(20 * c)
		r.skip(20 + 4)
	}
	// Section3b1: PrefPaths
	n, _ = r.list()
	for k := 0; k < n && r.err == nil; k++ {
		r.vecList()
		c, _ := r.list()
		r.skip(4 * c)
		r.skip(4 + 12 + 20)
	}
	// Section3b2: NavZones mit Gruppe
	n, m.sec3b2Count = r.list()
	zoneGUIDs := map[string]bool{}
	for k := 0; k < n && r.err == nil; k++ {
		name, g := r.proxy()
		if k == 0 && name != nil {
			m.zoneProxyName = append([]byte(nil), name...)
		}
		if g != nil {
			var id [16]byte
			copy(id[:], g)
			m.zones[id] = true
			zoneGUIDs[string(g)] = true
		}
		m.clusters[r.i32()] = true
		for l := 0; l < 3; l++ {
			c, _ := r.list()
			r.skip(4 * c)
		}
	}
	m.sec3b2End = r.pos
	if m.zoneProxyName == nil {
		return nil, errNavFormat
	}
	// Section3c: NegCircle-Ueberschneidungen
	n, _ = r.list()
	for k := 0; k < n && r.err == nil; k++ {
		r.skip(4)
		c, _ := r.list()
		for l := 0; l < c && r.err == nil; l++ {
			r.skip(20)
			cc, _ := r.list()
			r.skip(4 * cc)
		}
	}
	// Section3d: NavPaths mit Gruppe
	n, _ = r.list()
	for k := 0; k < n && r.err == nil; k++ {
		r.proxy()
		m.clusters[r.i32()] = true
		r.skip(6 * 12)
	}
	// Section3e: Zuordnung Interaktionsobjekte
	n, _ = r.list()
	for k := 0; k < n && r.err == nil; k++ {
		r.proxy()
		r.proxy()
	}
	// Section3fg: Wege, dann Zonen
	n, m.sec3fgCount = r.list()
	for k := 0; k < n && r.err == nil; k++ {
		r.proxy()
		r.skip(1)
		r.skip(1)
		c := r.u16()
		r.skip(4 * c)
	}
	m.sec3fgEnd = r.pos
	if r.err != nil {
		return nil, r.err
	}
	// Zelleneintraege: Wege erkennen (Name ungleich Zone)
	for x := range m.cells {
		for z := range m.cells[x] {
			for k := range m.cells[x][z].entries {
				e := &m.cells[x][z].entries[k]
				rr := &navReader{d: d, pos: e.off}
				name, _ := rr.proxy()
				e.isPath = name != nil && !bytes.Equal(name, m.zoneProxyName)
			}
		}
	}
	return m, nil
}

// HasZone meldet, ob die Zone schon eingetragen ist.
func (m *NavMap) HasZone(g [16]byte) bool { return m.zones[g] }

func (m *NavMap) zoneProxy(g [16]byte) []byte {
	b := []byte{1, 0, 1}
	b = append(b, m.zoneProxyName...)
	b = append(b, 1, 0, 1)
	b = append(b, g[:]...)
	return append(b, 0, 0, 0, 0)
}

// cellRange berechnet die beruehrten Rasterzellen (wie g3dit, in float32).
func (m *NavMap) cellRange(z NavZoneInfo) (x0, x1, z0, z1 int, ok bool) {
	cx, cz, r := z.Center[0], z.Center[2], z.Radius
	if cx-r < m.minX || cx+r > m.maxX || cz-r < m.minZ || cz+r > m.maxZ {
		return 0, 0, 0, 0, false
	}
	x0 = int(int32((cx - r - m.minX) / m.cellX))
	x1 = int(int32((cx + r - m.minX) / m.cellX))
	z0 = int(int32((cz - r - m.minZ) / m.cellZ))
	z1 = int(int32((cz + r - m.minZ) / m.cellZ))
	if x0 < 0 || z0 < 0 || x1 >= m.nx || z1 >= m.nz {
		return 0, 0, 0, 0, false
	}
	return x0, x1, z0, z1, true
}

// InGrid meldet, ob die Zone innerhalb des Kartenrasters liegt.
func (m *NavMap) InGrid(z NavZoneInfo) bool {
	_, _, _, _, ok := m.cellRange(z)
	return ok
}

type navEdit struct {
	off int
	ins []byte // einfuegen bei off
	inc int32  // oder: int32 bei off erhoehen
}

// AddZones traegt Zonen ein und liefert die neue Datei. Bereits eingetragene
// Zonen werden uebersprungen. Die Karte selbst bleibt unveraendert.
func (m *NavMap) AddZones(zones []NavZoneInfo) ([]byte, error) {
	var edits []navEdit
	used := map[int32]bool{}
	for c := range m.clusters {
		used[c] = true
	}
	var add3b2, add3fg []byte
	added := 0
	cellAdds := map[[2]int][]byte{}
	for _, z := range zones {
		if m.zones[z.GUID] {
			continue
		}
		x0, x1, z0, z1, ok := m.cellRange(z)
		if !ok {
			return nil, fmt.Errorf("Zone liegt außerhalb des Navigationsrasters")
		}
		p := m.zoneProxy(z.GUID)
		for x := x0; x <= x1; x++ {
			for zz := z0; zz <= z1; zz++ {
				k := [2]int{x, zz}
				cellAdds[k] = append(cellAdds[k], p...)
			}
		}
		// freie Gruppennummer wie g3dit (naechste Luecke nach 0)
		var list []int
		for c := range used {
			list = append(list, int(c))
		}
		sort.Ints(list)
		prev, next := 0, -1
		for _, v := range list {
			if v-prev > 1 {
				next = prev + 1
				break
			}
			prev = v
		}
		if next < 0 {
			next = prev + 1
		}
		used[int32(next)] = true
		add3b2 = append(add3b2, p...)
		add3b2 = binary.LittleEndian.AppendUint32(add3b2, uint32(next))
		for l := 0; l < 3; l++ {
			add3b2 = append(add3b2, 1, 0, 0, 0, 0)
		}
		add3fg = append(add3fg, p...)
		add3fg = append(add3fg, 0, 1, 0, 0)
		added++
	}
	if added == 0 {
		return append([]byte(nil), m.data...), nil
	}
	for k, b := range cellAdds {
		c := m.cells[k[0]][k[1]]
		at := c.end
		for _, e := range c.entries {
			if e.isPath {
				at = e.off
				break
			}
		}
		n := int32(len(b) / len(m.zoneProxy([16]byte{})))
		edits = append(edits, navEdit{off: c.countOff, inc: n}, navEdit{off: at, ins: b})
	}
	edits = append(edits,
		navEdit{off: m.sec3b2Count, inc: int32(added)}, navEdit{off: m.sec3b2End, ins: add3b2},
		navEdit{off: m.sec3fgCount, inc: int32(added)}, navEdit{off: m.sec3fgEnd, ins: add3fg})
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].off < edits[j].off })
	var out bytes.Buffer
	out.Grow(len(m.data) + 4096)
	pos, delta := 0, 0
	for _, e := range edits {
		out.Write(m.data[pos:e.off])
		pos = e.off
		if e.ins != nil {
			out.Write(e.ins)
			delta += len(e.ins)
		} else {
			v := int32(binary.LittleEndian.Uint32(m.data[e.off:])) + e.inc
			out.Write(binary.LittleEndian.AppendUint32(nil, uint32(v)))
			pos += 4
		}
	}
	out.Write(m.data[pos:])
	b := out.Bytes()
	// Verweis auf die Stringtabelle im Kopf
	st := binary.LittleEndian.Uint32(b[10:])
	binary.LittleEndian.PutUint32(b[10:], st+uint32(delta))
	// Probe: neu einlesen
	chk, err := ParseNavMap(b)
	if err != nil {
		return nil, fmt.Errorf("NavigationMap nach dem Eintragen ungültig: %v", err)
	}
	for _, z := range zones {
		if !chk.zones[z.GUID] {
			return nil, fmt.Errorf("NavigationMap: Zone fehlt nach dem Eintragen")
		}
	}
	return b, nil
}

// NavMapPath ist der Pfad der Navigationskarte im Archiv Projects_compiled.
const NavMapPath = "G3_World_01/NavigationMap.xnav"

// BaseFile liest eine Datei aus dem Spiel ohne Mods: nur <base>.pak und
// seine Patch-Generationen (.p00, .p01 ...), keine .mod/.m00-Archive.
func BaseFile(dataDir, base, name string) ([]byte, error) {
	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }
	var gens []string
	for _, p := range archiveGenerations(dataDir, base, exists) {
		ext := strings.ToLower(filepath.Ext(p))
		if ext == ".pak" || (len(ext) == 4 && ext[1] == 'p') {
			gens = append(gens, p)
		}
	}
	for i := len(gens) - 1; i >= 0; i-- {
		pk, err := OpenPak(gens[i])
		if err != nil {
			continue
		}
		for _, e := range pk.Entries {
			if strings.EqualFold(e.Name, name) && e.Size > 0 {
				d, err := e.Read()
				pk.Close()
				return d, err
			}
		}
		pk.Close()
	}
	return nil, fmt.Errorf("%s nicht gefunden", name)
}
