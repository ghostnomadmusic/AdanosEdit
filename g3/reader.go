package g3

import (
	"encoding/binary"
	"fmt"
	"math"
)

// parseError wird per panic geworfen und in den Parse-Funktionen abgefangen.
type parseError struct{ msg string }

func (e parseError) Error() string { return e.msg }

func fail(format string, a ...any) { panic(parseError{fmt.Sprintf(format, a...)}) }

// recoverParse wandelt einen parseError (oder Index-Fehler) in einen error um.
func recoverParse(err *error) {
	if r := recover(); r != nil {
		switch x := r.(type) {
		case parseError:
			*err = x
		case error:
			*err = fmt.Errorf("Lesefehler: %v", x)
		default:
			*err = fmt.Errorf("Lesefehler: %v", x)
		}
	}
}

// reader liest Genome-Binaerdaten. Ist table != nil, werden Strings als
// Index in die Stringtabelle gelesen, sonst inline (u16 Laenge + Zeichen).
type reader struct {
	d     []byte
	p     int
	table []string
	// von entities() gesetzt
	countOff, linksStart, linksEnd int
	links                          [][2]int32
}

func (r *reader) need(n int) {
	if n < 0 || r.p+n > len(r.d) {
		fail("Datei endet unerwartet (Position %d, benoetigt %d)", r.p, n)
	}
}
func (r *reader) skip(n int) { r.need(n); r.p += n }
func (r *reader) u8() byte   { r.need(1); v := r.d[r.p]; r.p++; return v }
func (r *reader) u16() uint16 {
	r.need(2)
	v := binary.LittleEndian.Uint16(r.d[r.p:])
	r.p += 2
	return v
}
func (r *reader) u32() uint32 {
	r.need(4)
	v := binary.LittleEndian.Uint32(r.d[r.p:])
	r.p += 4
	return v
}
func (r *reader) i32() int32 { return int32(r.u32()) }
func (r *reader) f32() float32 {
	return math.Float32frombits(r.u32())
}
func (r *reader) floats(dst []float32) {
	for i := range dst {
		dst[i] = r.f32()
	}
}
func (r *reader) bytes(n int) []byte {
	r.need(n)
	b := r.d[r.p : r.p+n]
	r.p += n
	return b
}
func (r *reader) entry() string {
	if r.table == nil {
		n := int(r.u16())
		return string(r.bytes(n))
	}
	i := int(r.u16())
	if i >= len(r.table) {
		fail("String-Index %d ausserhalb der Tabelle (%d)", i, len(r.table))
	}
	return r.table[i]
}

// genome oeffnet eine Datei mit GENOMFLE-Kopf und liest die Stringtabelle.
// Ohne Kopf wird ein Reader mit Inline-Strings geliefert.
func genome(d []byte) (*reader, bool) {
	if len(d) < 14 || string(d[:8]) != "GENOMFLE" {
		return &reader{d: d}, false
	}
	r := &reader{d: d, p: 8}
	r.u16()
	tabOff := int(r.u32())
	t := &reader{d: d, p: tabOff + 4}
	var table []string
	if tabOff > 0 && tabOff+5 <= len(d) && t.u8() != 0 {
		n := int(t.u32())
		if n > len(d) {
			fail("Stringtabelle defekt")
		}
		table = make([]string, 0, n)
		for i := 0; i < n; i++ {
			l := int(t.u16())
			table = append(table, string(t.bytes(l)))
		}
	}
	if table == nil {
		table = []string{}
	}
	r.table = table
	r.d = d[:max(tabOff, r.p)]
	return r, true
}

// Property ist eine serialisierte Eigenschaft (Name, Typ, Rohdaten).
type Property struct {
	Name, Type string
	Data       []byte
}

// PropSet ist ein Property-Set (z. B. eCVisualMeshStatic_PS) einer Entity.
type PropSet struct {
	Class string
	Props []Property
}

// Get liefert eine Eigenschaft per Name.
func (ps *PropSet) Get(name string) *Property {
	for i := range ps.Props {
		if ps.Props[i].Name == name {
			return &ps.Props[i]
		}
	}
	return nil
}

// propsBody liest den gemeinsamen Teil eines Genome-Objekts
// (Version, Groesse, Eigenschaften). Liefert das Ende des Objekts.
func (r *reader) propsBody(ps *PropSet) (end int) {
	ver := r.u16()
	size := int(r.u32())
	end = r.p + size
	if ver == 1 {
		r.entry()
	}
	if ver <= 0x51 {
		r.skip(20)
	}
	r.u16()
	n := int(r.u32())
	if n > 100000 {
		fail("unplausible Anzahl Eigenschaften: %d", n)
	}
	for i := 0; i < n; i++ {
		var p Property
		p.Name = r.entry()
		p.Type = r.entry()
		r.u16()
		sz := int(r.u32())
		p.Data = r.bytes(sz)
		ps.Props = append(ps.Props, p)
	}
	return end
}

// stringValue dekodiert eine String-Eigenschaft (bCString, bCMeshResourceString ...).
func (r *reader) stringValue(p *Property) string {
	if p == nil {
		return ""
	}
	if r.table == nil {
		if len(p.Data) < 2 {
			return ""
		}
		n := int(binary.LittleEndian.Uint16(p.Data))
		if 2+n > len(p.Data) {
			return ""
		}
		return string(p.Data[2 : 2+n])
	}
	if len(p.Data) < 2 {
		return ""
	}
	i := int(binary.LittleEndian.Uint16(p.Data))
	if i < len(r.table) {
		return r.table[i]
	}
	return ""
}

func intValue(p *Property) int {
	if p == nil || len(p.Data) < 4 {
		return 0
	}
	return int(int32(binary.LittleEndian.Uint32(p.Data)))
}
