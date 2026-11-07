package g3

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

// DumpClass gibt ein Property-Set (Welt-Format, ab Unterklassen-Kopf) lesbar aus (Diagnose).
func DumpClass(b []byte, table []string) (out string) {
	var sb strings.Builder
	defer func() {
		if r := recover(); r != nil {
			sb.WriteString(fmt.Sprintf("  !! %v\n", r))
		}
		out = sb.String()
	}()
	s := &entryScan{d: b, table: table}
	dumpSub(s, &sb, "")
	return
}

func dumpSub(s *entryScan, sb *strings.Builder, ind string) {
	s.skip(6)
	name := s.entry()
	s.skip(5)
	dumpBody(s, sb, ind, name)
}

func propValue(s *entryScan, typ string, d []byte) string {
	switch {
	case stringTypes[typ]:
		if len(d) >= 2 {
			i := int(binary.LittleEndian.Uint16(d))
			if i < len(s.table) {
				return fmt.Sprintf("%q", s.table[i])
			}
		}
	case typ == "bool" && len(d) == 1:
		return fmt.Sprint(d[0] != 0)
	case (typ == "int" || typ == "long" || typ == "unsigned long") && len(d) == 4:
		return fmt.Sprint(int32(binary.LittleEndian.Uint32(d)))
	case typ == "short" && len(d) == 2:
		return fmt.Sprint(int16(binary.LittleEndian.Uint16(d)))
	case typ == "float" && len(d) == 4:
		return fmt.Sprint(math.Float32frombits(binary.LittleEndian.Uint32(d)))
	case strings.HasPrefix(typ, "bTPropertyContainer<") && len(d) == 6:
		return fmt.Sprintf("enum %d", int32(binary.LittleEndian.Uint32(d[2:])))
	case (typ == "eCTemplateEntityProxy" || typ == "eCEntityProxy") && len(d) >= 3:
		if d[2] == 0 {
			return "(leer)"
		}
		return fmt.Sprintf("guid %x", d[3:min(len(d), 19)])
	}
	if len(d) > 24 {
		return fmt.Sprintf("[%d] %x…", len(d), d[:24])
	}
	return fmt.Sprintf("[%d] %x", len(d), d)
}

func dumpBody(s *entryScan, sb *strings.Builder, ind, name string) {
	ver := s.u16()
	size := int(s.u32())
	end := s.p + size
	if ver == 1 {
		s.entry()
	}
	if ver <= 0x51 {
		s.skip(20)
	}
	s.u16()
	n := int(s.u32())
	fmt.Fprintf(sb, "%s%s\n", ind, name)
	for i := 0; i < n; i++ {
		pn := s.entry()
		pt := s.entry()
		s.u16()
		sz := int(s.u32())
		d := s.d[s.p : s.p+sz]
		if strings.HasPrefix(pt, "bTPropertyObject<class ") {
			cls := strings.TrimPrefix(pt, "bTPropertyObject<class ")
			if j := strings.IndexAny(cls, ",>"); j >= 0 {
				cls = strings.TrimSpace(cls[:j])
			}
			fmt.Fprintf(sb, "%s  %s:\n", ind, pn)
			dumpBody(s, sb, ind+"    ", cls)
			continue
		}
		fmt.Fprintf(sb, "%s  %s (%s) = %s\n", ind, pn, pt, propValue(s, pt, d))
		s.p += sz
	}
	cv := s.u16()
	if s.p < end {
		switch name {
		case "gCInventory_PS":
			s.u16()
			k := int(s.u32())
			fmt.Fprintf(sb, "%s  [Stapel: %d]\n", ind, k)
			for i := 0; i < k; i++ {
				dumpSub(s, sb, ind+"    ")
			}
			s.u16()
			slots := int(s.u32())
			for i := 0; i < slots; i++ {
				if s.d[s.p+2] == 1 {
					fmt.Fprintf(sb, "%s  [Slot %d]\n", ind, i)
					dumpSub(s, sb, ind+"    ")
				} else {
					s.skip(3)
				}
			}
		case "eCCollisionShape_PS":
			k := int(s.u32())
			for i := 0; i < k; i++ {
				dumpSub(s, sb, ind+"    ")
			}
		}
		if s.p < end {
			fmt.Fprintf(sb, "%s  [post v%d: %d Byte] %x\n", ind, cv, end-s.p, s.d[s.p:min(end, s.p+40)])
		}
	}
	s.p = end
}
