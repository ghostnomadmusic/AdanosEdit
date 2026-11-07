package g3

// Bearbeiten von Property-Sets (Klassen) auf Byte-Ebene: Eigenschaften lesen,
// ersetzen (auch mit anderer Laenge) und das Set neu zusammensetzen.

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Prop ist eine Eigenschaft eines Property-Sets.
type Prop struct {
	NameIdx, TypeIdx uint16
	Name, Type       string
	Flag             uint16 // immer 0x1e
	Data             []byte
}

// ClassBody ist ein zerlegtes Property-Set (ab Unterklassen-Kopf).
type ClassBody struct {
	Name  string
	pre   []byte // Unterklassen-Kopf + Version (vor dem Groessenfeld)
	mid   []byte // optionaler Teil + Property-Version (nach dem Groessenfeld)
	Props []Prop
	Post  []byte // Klassenversion + Zusatzdaten
}

// ParseClassBody zerlegt ein Property-Set.
func ParseClassBody(b []byte, table []string) (cb *ClassBody, err error) {
	defer recoverParse(&err)
	s := &entryScan{d: b, table: table}
	s.skip(6)
	name := s.entry()
	s.skip(5)
	ver := s.u16()
	sizeOff := s.p
	size := int(s.u32())
	end := s.p + size
	if end > len(b) {
		fail("Klasse %s: Groesse passt nicht", name)
	}
	cb = &ClassBody{Name: name, pre: append([]byte(nil), b[:sizeOff]...)}
	ms := s.p
	if ver == 1 {
		s.entry()
	}
	if ver <= 0x51 {
		s.skip(20)
	}
	s.u16()
	cb.mid = append([]byte(nil), b[ms:s.p]...)
	n := int(s.u32())
	for i := 0; i < n; i++ {
		var p Prop
		p.NameIdx = binary.LittleEndian.Uint16(s.d[s.p:])
		p.Name = s.entry()
		p.TypeIdx = binary.LittleEndian.Uint16(s.d[s.p:])
		p.Type = s.entry()
		p.Flag = s.u16()
		sz := int(s.u32())
		s.need(sz)
		p.Data = append([]byte(nil), s.d[s.p:s.p+sz]...)
		s.p += sz
		cb.Props = append(cb.Props, p)
	}
	cb.Post = append([]byte(nil), b[s.p:end]...)
	if end != len(b) {
		fail("Klasse %s: %d Byte nach dem Ende", name, len(b)-end)
	}
	return cb, nil
}

// Bytes setzt das Property-Set wieder zusammen.
func (cb *ClassBody) Bytes() []byte {
	var body bytes.Buffer
	body.Write(cb.mid)
	binary.Write(&body, binary.LittleEndian, uint32(len(cb.Props)))
	for _, p := range cb.Props {
		binary.Write(&body, binary.LittleEndian, p.NameIdx)
		binary.Write(&body, binary.LittleEndian, p.TypeIdx)
		binary.Write(&body, binary.LittleEndian, p.Flag)
		binary.Write(&body, binary.LittleEndian, uint32(len(p.Data)))
		body.Write(p.Data)
	}
	body.Write(cb.Post)
	var out bytes.Buffer
	out.Write(cb.pre)
	binary.Write(&out, binary.LittleEndian, uint32(body.Len()))
	out.Write(body.Bytes())
	return out.Bytes()
}

// Get liefert eine Eigenschaft per Name.
func (cb *ClassBody) Get(name string) *Prop {
	for i := range cb.Props {
		if cb.Props[i].Name == name {
			return &cb.Props[i]
		}
	}
	return nil
}

// --- Werte kodieren ---

// ProxyData kodiert einen (Template-)Entity-Proxy. ver sind die ersten zwei
// Bytes des bisherigen Werts. Eine Null-GUID ergibt einen leeren Proxy.
func ProxyData(old []byte, guid [16]byte) []byte {
	ver := []byte{1, 0}
	if len(old) >= 2 {
		ver = old[:2]
	}
	if guid == ([16]byte{}) {
		return append(append([]byte(nil), ver...), 0)
	}
	out := append(append([]byte(nil), ver...), 1)
	out = append(out, guid[:]...)
	return append(out, 0, 0, 0, 0)
}

// ProxyGUID liest die GUID eines Proxys (Null-GUID wenn leer).
func ProxyGUID(d []byte) (g [16]byte) {
	if len(d) >= 19 && d[2] != 0 {
		copy(g[:], d[3:19])
	}
	return
}

// IDData kodiert eine bCPropertyID (GUID + 4 Byte).
func IDData(guid [16]byte) []byte {
	return append(append([]byte(nil), guid[:]...), 0, 0, 0, 0)
}

// IDArrayData kodiert ein bTValArray<class bCPropertyID>.
func IDArrayData(ids ...[16]byte) []byte {
	out := []byte{1}
	out = binary.LittleEndian.AppendUint32(out, uint32(len(ids)))
	for _, g := range ids {
		out = append(out, IDData(g)...)
	}
	return out
}

// EnumData kodiert einen bTPropertyContainer<enum ...>.
func EnumData(old []byte, v int32) []byte {
	ver := []byte{1, 0}
	if len(old) >= 2 {
		ver = old[:2]
	}
	return binary.LittleEndian.AppendUint32(append([]byte(nil), ver...), uint32(v))
}

// --- Inventar (gCInventory_PS) ---

// InvSlot ist ein Ausruestungsplatz (Kopf, Koerper, Haare ...).
type InvSlot struct {
	Present bool
	Body    *ClassBody // gCInventorySlot
}

// Inventory ist der Zusatzteil von gCInventory_PS.
type Inventory struct {
	cv, v1 []byte
	stacks []byte // Anzahl + Stapel (unveraendert)
	v2     []byte
	Slots  []InvSlot
	tail   []byte // 5 Proxys
}

// ParseInventory zerlegt den Zusatzteil (cb.Post) eines gCInventory_PS.
func ParseInventory(post []byte, table []string) (inv *Inventory, err error) {
	defer recoverParse(&err)
	s := &entryScan{d: post, table: table}
	inv = &Inventory{}
	inv.cv = post[0:2]
	s.p = 2
	inv.v1 = post[s.p : s.p+2]
	s.p += 2
	st := s.p
	n := int(s.u32())
	for i := 0; i < n; i++ {
		s.skip(6)
		s.entry()
		s.skip(5)
		s.u16()
		s.skip(int(s.u32()))
	}
	inv.stacks = post[st:s.p]
	inv.v2 = post[s.p : s.p+2]
	s.p += 2
	k := int(s.u32())
	for i := 0; i < k; i++ {
		s.need(3)
		if s.d[s.p+2] == 1 {
			a := s.p
			s.skip(6)
			s.entry()
			s.skip(5)
			s.u16()
			s.skip(int(s.u32()))
			cb, e := ParseClassBody(post[a:s.p], table)
			if e != nil {
				fail("Inventarplatz %d: %v", i, e)
			}
			inv.Slots = append(inv.Slots, InvSlot{Present: true, Body: cb})
		} else {
			s.skip(3)
			inv.Slots = append(inv.Slots, InvSlot{})
		}
	}
	inv.tail = post[s.p:]
	return inv, nil
}

// Bytes setzt den Zusatzteil wieder zusammen.
func (inv *Inventory) Bytes() []byte {
	var b bytes.Buffer
	b.Write(inv.cv)
	b.Write(inv.v1)
	b.Write(inv.stacks)
	b.Write(inv.v2)
	binary.Write(&b, binary.LittleEndian, uint32(len(inv.Slots)))
	for _, s := range inv.Slots {
		if s.Present {
			b.Write(s.Body.Bytes())
		} else {
			b.Write([]byte{1, 0, 0})
		}
	}
	b.Write(inv.tail)
	return b.Bytes()
}

// Prototype liefert einen vorhandenen Platz als Vorlage fuer neue Plaetze.
func (inv *Inventory) Prototype() *ClassBody {
	for _, s := range inv.Slots {
		if s.Present {
			return s.Body
		}
	}
	return nil
}

// SetSlot belegt einen Platz (Vorlage tpl, Gegenstand item; Null-GUID tpl = leeren).
func (inv *Inventory) SetSlot(k int, tpl, item [16]byte, slotEnum int32) error {
	if k < 0 || k >= len(inv.Slots) {
		return fmt.Errorf("ungueltiger Inventarplatz %d", k)
	}
	if tpl == ([16]byte{}) {
		inv.Slots[k] = InvSlot{}
		return nil
	}
	var cb *ClassBody
	if inv.Slots[k].Present {
		cb = inv.Slots[k].Body
	} else {
		p := inv.Prototype()
		if p == nil {
			return fmt.Errorf("kein Vorbild fuer Inventarplaetze")
		}
		c := *p
		c.Props = append([]Prop(nil), p.Props...)
		cb = &c
	}
	for i := range cb.Props {
		p := &cb.Props[i]
		switch p.Name {
		case "Template":
			p.Data = ProxyData(p.Data, tpl)
		case "Item":
			p.Data = ProxyData(p.Data, item)
		case "Slot":
			p.Data = EnumData(p.Data, slotEnum)
		}
	}
	inv.Slots[k] = InvSlot{Present: true, Body: cb}
	return nil
}

// StackBytes liefert die (unveraenderten) Stapel-Daten: Anzahl + Stapel.
func (inv *Inventory) StackBytes() []byte { return inv.stacks }

// StackItem ist ein Stapel Gegenstaende im Inventar (Vorlage + Anzahl).
type StackItem struct {
	Template [16]byte
	Amount   int
}

// Stacks liest die Gegenstaende im Inventar.
func (inv *Inventory) Stacks(table []string) (items []StackItem, err error) {
	defer recoverParse(&err)
	s := &entryScan{d: inv.stacks, table: table}
	n := int(s.u32())
	for i := 0; i < n; i++ {
		a := s.p
		s.skip(6)
		s.entry()
		s.skip(5)
		s.u16()
		s.skip(int(s.u32()))
		cb, e := ParseClassBody(inv.stacks[a:s.p], table)
		if e != nil {
			return nil, e
		}
		var it StackItem
		if p := cb.Get("Template"); p != nil {
			it.Template = ProxyGUID(p.Data)
		}
		if p := cb.Get("Amount"); p != nil && len(p.Data) >= 4 {
			it.Amount = int(int32(binary.LittleEndian.Uint32(p.Data)))
		}
		items = append(items, it)
	}
	return items, nil
}

// SetStacks ersetzt die Gegenstaende im Inventar. intern liefert den Index
// eines Namens in der Stringtabelle der Datei.
func (inv *Inventory) SetStacks(items []StackItem, intern func(string) uint16) {
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, uint32(len(items)))
	u16 := func(v uint16) { binary.Write(&b, binary.LittleEndian, v) }
	for _, it := range items {
		type pr struct {
			name, typ string
			data      []byte
		}
		i32 := func(v int32) []byte { return binary.LittleEndian.AppendUint32(nil, uint32(v)) }
		props := []pr{
			{"Amount", "int", i32(int32(it.Amount))},
			{"Quality", "int", i32(0)},
			{"QuickSlot", "int", i32(-1)},
			{"SortIndex", "int", i32(-1)},
			{"Generated", "bTPropertyContainer<enum gEStackType>", []byte{1, 0, 0, 0, 0, 0}},
			{"Learned", "bool", []byte{0}},
			{"ActivationCount", "int", i32(0)},
			{"TransactionCount", "int", i32(0)},
			{"Template", "eCTemplateEntityProxy", ProxyData(nil, it.Template)},
			{"Item", "eCEntityProxy", []byte{1, 0, 0}},
			{"Slot", "bTPropertyContainer<enum gESlot>", []byte{1, 0, 0, 0, 0, 0}},
		}
		var body bytes.Buffer
		binary.Write(&body, binary.LittleEndian, uint16(0x1e))
		binary.Write(&body, binary.LittleEndian, uint32(len(props)))
		for _, p := range props {
			binary.Write(&body, binary.LittleEndian, intern(p.name))
			binary.Write(&body, binary.LittleEndian, intern(p.typ))
			binary.Write(&body, binary.LittleEndian, uint16(0x1e))
			binary.Write(&body, binary.LittleEndian, uint32(len(p.data)))
			body.Write(p.data)
		}
		body.Write([]byte{1, 0})
		b.Write([]byte{1, 0, 1, 1, 0, 1})
		u16(intern("gCInventoryStack"))
		b.Write([]byte{1, 0, 0, 0x53, 0})
		u16(0x53)
		binary.Write(&b, binary.LittleEndian, uint32(body.Len()))
		b.Write(body.Bytes())
	}
	inv.stacks = b.Bytes()
}
