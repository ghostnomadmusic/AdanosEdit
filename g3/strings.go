package g3

// stringtable.ini (UTF-16LE, alle Sprachen in einer Zeile).

import (
	"encoding/binary"
	"strings"
	"unicode/utf16"
)

// Sprachen in der Reihenfolge der stringtable.ini
var StringLangs = []string{"English", "Italian", "French", "German", "Spanish", "Czech", "Hungarian", "Polish", "Russian", "TRC"}

func decodeUTF16(b []byte) string {
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		b = b[2:]
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[i*2:])
	}
	return string(utf16.Decode(u))
}

func encodeUTF16(s string) []byte {
	u := utf16.Encode([]rune(s))
	out := make([]byte, 2+len(u)*2)
	out[0], out[1] = 0xFF, 0xFE
	for i, c := range u {
		binary.LittleEndian.PutUint16(out[2+i*2:], c)
	}
	return out
}

// StringEntry liest einen Eintrag (Texte je Sprache) aus der stringtable.ini.
func StringEntry(data []byte, key string) ([]string, bool) {
	s := decodeUTF16(data)
	p := "\n" + key + "="
	i := strings.Index(s, p)
	if i < 0 {
		return nil, false
	}
	line := s[i+len(p):]
	if j := strings.IndexAny(line, "\r\n"); j >= 0 {
		line = line[:j]
	}
	parts := strings.Split(strings.TrimSuffix(line, ";"), ";;")
	return parts, true
}

// SetStringEntry setzt (oder ergaenzt) einen Eintrag in der stringtable.ini.
// texts sind die Texte je Sprache (siehe StringLangs); fehlende werden mit
// dem ersten Text aufgefuellt.
// StringValue bildet den Wert einer Zeile (Texte je Sprache, ";;"-getrennt).
func StringValue(texts []string) string {
	vals := make([]string, len(StringLangs))
	for i := range vals {
		if i < len(texts) && texts[i] != "" {
			vals[i] = texts[i]
		} else if len(texts) > 0 {
			vals[i] = texts[0]
		}
		vals[i] = strings.NewReplacer(";", ",", "\r", " ", "\n", " ").Replace(vals[i])
	}
	return strings.Join(vals, ";;") + ";"
}

func SetStringEntry(data []byte, key string, texts []string) []byte {
	s := decodeUTF16(data)
	line := key + "=" + StringValue(texts)
	p := "\n" + key + "="
	if i := strings.Index(s, p); i >= 0 {
		j := strings.IndexAny(s[i+1:], "\r\n")
		if j < 0 {
			j = len(s) - i - 1
		}
		s = s[:i+1] + line + s[i+1+j:]
		return encodeUTF16(s)
	}
	// am Ende des Abschnitts [LocAdmin_Strings] einfuegen
	sec := strings.Index(s, "[LocAdmin_Revisions]")
	if sec < 0 {
		if !strings.HasSuffix(s, "\n") {
			s += "\r\n"
		}
		return encodeUTF16(s + line + "\r\n")
	}
	head := strings.TrimRight(s[:sec], "\r\n")
	return encodeUTF16(head + "\r\n" + line + "\r\n" + s[sec:])
}

// StringValues liefert alle Eintraege (Schluessel -> Text hinter dem "=")
// einer stringtable.ini oder stringtableMod.ini.
func StringValues(data []byte) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(decodeUTF16(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "[") || strings.HasPrefix(line, ";") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok && k != "" {
			m[k] = v
		}
	}
	return m
}

// StringtableMod erzeugt eine stringtableMod.ini fuer den Mod Starter
// (UTF-16LE mit BOM wie die stringtable.ini, Abschnitt [LocAdmin_Strings]).
func StringtableMod(vals map[string]string) []byte {
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sortStrings(keys)
	var b strings.Builder
	b.WriteString("[LocAdmin_Strings]\r\n")
	for _, k := range keys {
		b.WriteString(k + "=" + vals[k] + "\r\n")
	}
	return encodeUTF16(b.String())
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
