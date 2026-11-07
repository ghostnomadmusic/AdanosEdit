package g3

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Lock beschreibt das Schloss einer Truhe oder Tuer (gCLock_PS).
type Lock struct {
	Locked     bool
	Difficulty int      // 0 = nur mit Schluessel, 1..3 = leicht..schwer zu knacken (offen: -1)
	Key        [16]byte // Vorlage des Schluessels (leer = keiner)
}

// HasClass prueft, ob eine Entity ein Property-Set hat.
func (wf *WorldFile) HasClass(i int, name string) bool {
	_, err := wf.EntityClass(i, name)
	return err == nil
}

// LockOf liest das Schloss.
func (wf *WorldFile) LockOf(i int) (Lock, error) {
	var l Lock
	cb, err := wf.EntityClass(i, "gCLock_PS")
	if err != nil {
		return l, err
	}
	for _, p := range cb.Props {
		switch p.Name {
		case "Status":
			if len(p.Data) >= 6 {
				l.Locked = binary.LittleEndian.Uint32(p.Data[2:]) == 0
			}
		case "Difficulty":
			if len(p.Data) >= 4 {
				l.Difficulty = int(int32(binary.LittleEndian.Uint32(p.Data)))
			}
		case "Key":
			if len(p.Data) >= 19 && p.Data[2] == 1 {
				copy(l.Key[:], p.Data[3:19])
			}
		}
	}
	return l, nil
}

// SetLock schreibt das Schloss.
func (wf *WorldFile) SetLock(i int, l Lock) error {
	cb, err := wf.EntityClass(i, "gCLock_PS")
	if err != nil {
		return err
	}
	for k := range cb.Props {
		p := &cb.Props[k]
		switch p.Name {
		case "Status":
			v := int32(1)
			if l.Locked {
				v = 0
			}
			p.Data = EnumData(p.Data, v)
		case "Difficulty":
			v := int32(l.Difficulty)
			if !l.Locked && v != 0 {
				v = -1 // offen
			}
			p.Data = binary.LittleEndian.AppendUint32(nil, uint32(v))
		case "Key":
			if l.Key == ([16]byte{}) {
				p.Data = []byte{1, 0, 0}
			} else {
				p.Data = append(append([]byte{1, 0, 1}, l.Key[:]...), 0, 0, 0, 0)
			}
		}
	}
	return wf.SetEntityClass(i, cb)
}

// ContainerSets liest die Schatzsets eines Behaelters (Truhe).
func (wf *WorldFile) ContainerSets(i int) ([]string, error) {
	cb, err := wf.EntityClass(i, "gCInventory_PS")
	if err != nil {
		return nil, err
	}
	var out []string
	for n := 1; n <= 5; n++ {
		if p := cb.Get(fmt.Sprintf("TreasureSet%d", n)); p != nil {
			if s := wf.StrValue(p.Data); s != "" {
				out = append(out, s)
			}
		}
	}
	return out, nil
}

// SetContainerSets setzt die Schatzsets eines Behaelters (hoechstens 5).
func (wf *WorldFile) SetContainerSets(i int, sets []string) error {
	if len(sets) > 5 {
		return fmt.Errorf("zu viele Schatzsets (%d, hoechstens 5)", len(sets))
	}
	cb, err := wf.EntityClass(i, "gCInventory_PS")
	if err != nil {
		return err
	}
	for n := 1; n <= 5; n++ {
		v := ""
		if n-1 < len(sets) {
			v = sets[n-1]
		}
		if p := cb.Get(fmt.Sprintf("TreasureSet%d", n)); p != nil {
			p.Data = wf.StrData(v)
		}
	}
	if p := cb.Get("GeneratedPlunder"); p != nil {
		p.Data = []byte{0}
	}
	return wf.SetEntityClass(i, cb)
}

// ZoneShape liest den Umriss einer Zone (gCNavZone_PS): Eckpunkte relativ
// zur Entity, Hoehe nach oben und unten.
func (wf *WorldFile) ZoneShape(i int) (pts [][3]float32, radius, top, bottom float32, err error) {
	cb, err := wf.EntityClass(i, "gCNavZone_PS")
	if err != nil {
		return nil, 0, 0, 0, err
	}
	f32 := func(b []byte) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(b)) }
	for _, p := range cb.Props {
		switch p.Name {
		case "Point":
			if len(p.Data) >= 5 {
				n := int(binary.LittleEndian.Uint32(p.Data[1:]))
				for k := 0; k < n && 5+k*12+12 <= len(p.Data); k++ {
					o := 5 + k*12
					pts = append(pts, [3]float32{f32(p.Data[o:]), f32(p.Data[o+4:]), f32(p.Data[o+8:])})
				}
			}
		case "Radius":
			if len(p.Data) >= 4 {
				radius = f32(p.Data)
			}
		case "TopToleranz":
			if len(p.Data) >= 4 {
				top = f32(p.Data)
			}
		case "BottomToleranz":
			if len(p.Data) >= 4 {
				bottom = f32(p.Data)
			}
		}
	}
	return pts, radius, top, bottom, nil
}

// SetZoneShape setzt Umriss (Punkte relativ zur Entity), Radius und Hoehe einer Zone.
func (wf *WorldFile) SetZoneShape(i int, pts [][3]float32, radius, top, bottom float32) error {
	cb, err := wf.EntityClass(i, "gCNavZone_PS")
	if err != nil {
		return err
	}
	f := func(v float32) []byte { return binary.LittleEndian.AppendUint32(nil, math.Float32bits(v)) }
	for k := range cb.Props {
		p := &cb.Props[k]
		switch p.Name {
		case "Point":
			d := binary.LittleEndian.AppendUint32([]byte{1}, uint32(len(pts)))
			for _, q := range pts {
				d = append(d, f(q[0])...)
				d = append(d, f(q[1])...)
				d = append(d, f(q[2])...)
			}
			p.Data = d
		case "Radius":
			p.Data = f(radius)
		case "RadiusOffset":
			p.Data = append(append(f(0), f(0)...), f(0)...)
		case "TopToleranz":
			p.Data = f(top)
		case "BottomToleranz":
			p.Data = f(bottom)
		case "ZoneIsCCW":
			p.Data = []byte{1}
		}
	}
	if err := wf.SetEntityClass(i, cb); err != nil {
		return err
	}
	// Begrenzungsbox = Ausdehnung der Zone (das Spiel sucht Zonen ueber diese Box)
	var mn, mx [3]float32
	for k, q := range pts {
		for a := 0; a < 3; a++ {
			if k == 0 || q[a] < mn[a] {
				mn[a] = q[a]
			}
			if k == 0 || q[a] > mx[a] {
				mx[a] = q[a]
			}
		}
	}
	if len(pts) == 0 {
		mn, mx = [3]float32{-radius, 0, -radius}, [3]float32{radius, 0, radius}
	}
	h := float32(300)
	if top > h {
		h = top
	}
	if bottom > h {
		h = bottom
	}
	mn[1], mx[1] = mn[1]-h, mx[1]+h
	wf.setBox(i, 1, mn, mx)
	wf.SetWorld(i, wf.Ents[i].World)
	return nil
}

// ZoneCircle liefert Mittelpunkt (relativ zur Entity) und Radius einer Zone.
func (wf *WorldFile) ZoneCircle(i int) (offset [3]float32, radius float32, err error) {
	cb, err := wf.EntityClass(i, "gCNavZone_PS")
	if err != nil {
		return offset, 0, err
	}
	f32 := func(b []byte) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(b)) }
	for _, p := range cb.Props {
		switch p.Name {
		case "Radius":
			if len(p.Data) >= 4 {
				radius = f32(p.Data)
			}
		case "RadiusOffset":
			if len(p.Data) >= 12 {
				offset = [3]float32{f32(p.Data), f32(p.Data[4:]), f32(p.Data[8:])}
			}
		}
	}
	return offset, radius, nil
}
