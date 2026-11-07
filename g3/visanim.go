package g3

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// MatSlot ist ein Koerperteil-Eintrag in eCVisualAnimation_PS (Slot-Name, FXA, Materialvariante).
type MatSlot struct {
	Name   string // z. B. Slot_Head (leer beim Hauptmodell)
	Ver    uint16
	FXA    string
	Switch int32
	Has2   bool
	Ver2   uint16
	FXA2   string
	Sw2    int32
}

// VisAnim ist der Zusatzteil von eCVisualAnimation_PS.
type VisAnim struct {
	cv, v    []byte
	Main     MatSlot
	Parts    []MatSlot
	partFlag []byte
	flag     byte
	Entries  []string
	Extras   []VAExtra
	rest     []byte // Box, Basisklasse (unveraendert)
}

// VAExtra ist ein angehaengtes Teil (z. B. Haare, Bart) mit GUID der Kind-Entity.
type VAExtra struct {
	Ver     uint16
	HasGUID bool
	GUID    [20]byte
	Name    string
	Data    []byte // 72 Byte
}

func readMatSlot(s *entryScan, named bool) MatSlot {
	var m MatSlot
	if named {
		m.Name = s.entry()
	}
	m.Ver = s.u16()
	m.FXA = s.entry()
	m.Switch = int32(s.u32())
	if s.u8() != 0 {
		m.Has2 = true
		m.Ver2 = s.u16()
		m.FXA2 = s.entry()
		m.Sw2 = int32(s.u32())
	}
	return m
}

func writeMatSlot(b *bytes.Buffer, m MatSlot, named bool, idx func(string) uint16) {
	if named {
		binary.Write(b, binary.LittleEndian, idx(m.Name))
	}
	binary.Write(b, binary.LittleEndian, m.Ver)
	binary.Write(b, binary.LittleEndian, idx(m.FXA))
	binary.Write(b, binary.LittleEndian, m.Switch)
	if m.Has2 {
		b.WriteByte(1)
		binary.Write(b, binary.LittleEndian, m.Ver2)
		binary.Write(b, binary.LittleEndian, idx(m.FXA2))
		binary.Write(b, binary.LittleEndian, m.Sw2)
	} else {
		b.WriteByte(0)
	}
}

// ParseVisAnim zerlegt den Zusatzteil (cb.Post) von eCVisualAnimation_PS.
func ParseVisAnim(post []byte, table []string) (va *VisAnim, err error) {
	defer recoverParse(&err)
	s := &entryScan{d: post, table: table}
	va = &VisAnim{cv: post[0:2]}
	s.p = 2
	va.v = post[2:4]
	s.p = 4
	va.Main = readMatSlot(s, false)
	n := int(s.u32())
	for i := 0; i < n; i++ {
		va.Parts = append(va.Parts, readMatSlot(s, true))
		va.partFlag = append(va.partFlag, s.u8())
	}
	va.flag = s.u8()
	n = int(s.u32())
	for i := 0; i < n; i++ {
		va.Entries = append(va.Entries, s.entry())
	}
	n = int(s.u32())
	for i := 0; i < n; i++ {
		var x VAExtra
		x.Ver = s.u16()
		if s.u8() != 0 {
			x.HasGUID = true
			s.need(20)
			copy(x.GUID[:], s.d[s.p:s.p+20])
			s.p += 20
		}
		x.Name = s.entry()
		s.need(72)
		x.Data = append([]byte(nil), s.d[s.p:s.p+72]...)
		s.p += 72
		va.Extras = append(va.Extras, x)
	}
	va.rest = post[s.p:]
	return va, nil
}

// Bytes setzt den Zusatzteil wieder zusammen (Strings ueber idx).
func (va *VisAnim) Bytes(idx func(string) uint16) []byte {
	var b bytes.Buffer
	b.Write(va.cv)
	b.Write(va.v)
	writeMatSlot(&b, va.Main, false, idx)
	binary.Write(&b, binary.LittleEndian, uint32(len(va.Parts)))
	for i, p := range va.Parts {
		writeMatSlot(&b, p, true, idx)
		f := byte(1)
		if i < len(va.partFlag) {
			f = va.partFlag[i]
		}
		b.WriteByte(f)
	}
	b.WriteByte(va.flag)
	binary.Write(&b, binary.LittleEndian, uint32(len(va.Entries)))
	for _, e := range va.Entries {
		binary.Write(&b, binary.LittleEndian, idx(e))
	}
	binary.Write(&b, binary.LittleEndian, uint32(len(va.Extras)))
	for _, x := range va.Extras {
		binary.Write(&b, binary.LittleEndian, x.Ver)
		if x.HasGUID {
			b.WriteByte(1)
			b.Write(x.GUID[:])
		} else {
			b.WriteByte(0)
		}
		binary.Write(&b, binary.LittleEndian, idx(x.Name))
		b.Write(x.Data)
	}
	b.Write(va.rest)
	return b.Bytes()
}

func (m MatSlot) String() string {
	s := fmt.Sprintf("%s: %s/%d", m.Name, m.FXA, m.Switch)
	if m.Has2 {
		s += fmt.Sprintf(" + %s/%d", m.FXA2, m.Sw2)
	}
	return s
}
