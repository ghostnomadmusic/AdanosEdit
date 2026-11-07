// Package lang enthaelt die Uebersetzungen der Oberflaeche.
// Schluessel ist jeweils der deutsche Originaltext.
package lang

import (
	"regexp"
	"sort"
)

// Names sind die waehlbaren Sprachen (Reihenfolge = Index).
var Names = []string{"Deutsch", "English", "Русский", "Polski"}

var codes = []string{"de", "en", "ru", "pl"}

// Current ist die eingestellte Sprache (0 = Deutsch).
var Current = 0

// Set waehlt die Sprache per Index.
func Set(i int) {
	if i >= 0 && i < len(Names) {
		Current = i
	}
}

// SetCode waehlt die Sprache per Kuerzel (de, en, ru, pl).
func SetCode(c string) {
	for i, x := range codes {
		if x == c {
			Current = i
		}
	}
}

// Code liefert das Kuerzel der aktuellen Sprache.
func Code() string { return codes[Current] }

// T uebersetzt einen deutschen Text.
func T(s string) string {
	if Current == 0 {
		return s
	}
	if t, ok := table[s]; ok && t[Current-1] != "" {
		return t[Current-1]
	}
	return s
}

// English liefert die englische Fassung.
func English(s string) string {
	if t, ok := table[s]; ok && t[0] != "" {
		return t[0]
	}
	return s
}

// Has prueft, ob ein Text vollstaendig uebersetzt ist.
func Has(s string) bool {
	t, ok := table[s]
	return ok && t[0] != "" && t[1] != "" && t[2] != ""
}

// Count liefert die Zahl der Eintraege.
func Count() int { return len(table) }

// Keys liefert alle Schluessel.
func Keys() []string {
	var k []string
	for x := range table {
		k = append(k, x)
	}
	sort.Strings(k)
	return k
}

var verbRe = regexp.MustCompile(`%[-+ #0]*[0-9]*(\.[0-9]+)?[a-zA-Z%]`)

// FormatProblems prueft, ob Platzhalter (%s, %d ...) in allen Sprachen gleich sind.
func FormatProblems() []string {
	var out []string
	for k, t := range table {
		want := verbRe.FindAllString(k, -1)
		for i, x := range t {
			got := verbRe.FindAllString(x, -1)
			if len(got) != len(want) {
				out = append(out, codes[i+1]+": "+k)
				continue
			}
			for j := range got {
				if got[j] != want[j] {
					out = append(out, codes[i+1]+": "+k)
					break
				}
			}
		}
	}
	return out
}
