//go:build windows

package main

// Verzweigte Gespraeche (Unterauswahl ueber Parent / Thema), Bedingung
// "Held hat Gegenstaende" und Goldkosten eines Dialogs.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"g3viewer/g3"
)

// dlgChildren: Name (klein) -> Kind-Dialoge eines NPCs.
func (a *app) dlgOrdered(owner string) []dlgItem {
	d := &a.dlg
	var all []dlgItem
	names := map[string]bool{}
	for _, p := range d.byOwner[strings.ToLower(owner)] {
		op := d.paths[p]
		if op == "" {
			op = p
		}
		all = append(all, dlgItem{info: d.infos[p], path: op})
		names[strings.ToLower(d.infos[p].Name)] = true
	}
	sort.SliceStable(all, func(i, j int) bool {
		x, y := all[i].info, all[j].info
		if x.SortID != y.SortID {
			return x.SortID < y.SortID
		}
		return x.Name < y.Name
	})
	kids := map[string][]dlgItem{}
	var roots []dlgItem
	for _, it := range all {
		p := strings.ToLower(it.info.Parent)
		if p != "" && names[p] && !strings.EqualFold(it.info.Parent, it.info.Name) {
			kids[p] = append(kids[p], it)
		} else {
			roots = append(roots, it)
		}
	}
	var out []dlgItem
	seen := map[string]bool{}
	var walk func(it dlgItem, depth int)
	walk = func(it dlgItem, depth int) {
		k := strings.ToLower(it.info.Name)
		if seen[k] || depth > 8 {
			return
		}
		seen[k] = true
		it.depth = depth
		out = append(out, it)
		for _, c := range kids[k] {
			walk(c, depth+1)
		}
	}
	for _, r := range roots {
		walk(r, 0)
	}
	for _, it := range all { // Reste (z. B. Kreise)
		if !seen[strings.ToLower(it.info.Name)] {
			out = append(out, it)
		}
	}
	return out
}

// isDescendant: ist cand (direkt oder ueber andere) unter root eingehaengt?
func (a *app) isDescendant(cand, root string) bool {
	d := &a.dlg
	byName := map[string]*g3.Info{}
	for _, in := range d.infos {
		byName[strings.ToLower(in.Name)] = in
	}
	cur := strings.ToLower(cand)
	for i := 0; i < 20 && cur != ""; i++ {
		if cur == strings.ToLower(root) {
			return true
		}
		in := byName[cur]
		if in == nil {
			return false
		}
		cur = strings.ToLower(in.Parent)
	}
	return false
}

// fillDlgParents fuellt die Auswahl "Unter" (Dialoge desselben NPCs).
func (a *app) fillDlgParents(it *dlgItem) {
	d := &a.dlg
	d.parents = []string{""}
	items := []string{T("(keiner – Hauptauswahl)")}
	sel := 0
	for _, o := range a.dlgOrdered(it.info.Owner) {
		if o.info == it.info || a.isDescendant(o.info.Name, it.info.Name) {
			continue // nicht unter sich selbst oder eigene Unterdialoge
		}
		d.parents = append(d.parents, o.info.Name)
		items = append(items, strings.Repeat("   ", o.depth)+a.dlgTitle(o))
		if strings.EqualFold(o.info.Name, it.info.Parent) {
			sel = len(d.parents) - 1
		}
	}
	comboFill(d.parent, items, sel)
}

// heroItemsText zeigt die Gegenstands-Bedingung (nur die des Helden).
func heroItemsText(in *g3.Info) string {
	c := strings.Split(in.CondItemContainer, ";")
	it := strings.Split(in.CondItems, ";")
	am := strings.Split(in.CondItemAmounts, ";")
	var parts []string
	for i, name := range it {
		if strings.TrimSpace(name) == "" {
			continue
		}
		who := ""
		if i < len(c) {
			who = c[i]
		}
		n := "1"
		if i < len(am) && am[i] != "" {
			n = am[i]
		}
		s := name + " " + n
		if who != "" && !strings.EqualFold(who, "PC_Hero") {
			s = who + ":" + s // Behaelter eines anderen NPCs
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "; ")
}

// parseHeroItems liest "It_Gold 100; It_Apple 2" (optional "NPC:It_X 1").
func (a *app) parseHeroItems(s string) (cont, items, amounts, msg string) {
	var c, it, am []string
	known := a.itemNames()
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		who := "PC_Hero"
		if k, v, ok := strings.Cut(part, ":"); ok {
			who, part = strings.TrimSpace(k), strings.TrimSpace(v)
		}
		f := strings.Fields(part)
		n := 1
		if len(f) > 1 {
			v, err := strconv.Atoi(f[1])
			if err != nil || v < 1 {
				return "", "", "", fmt.Sprintf(T("„%s“ ist keine gültige Zahl."), f[1])
			}
			n = v
		}
		if known != nil && !known[strings.ToLower(f[0])] {
			return "", "", "", fmt.Sprintf(T("Den Gegenstand „%s“ kenne ich nicht (Vorlagenname wie It_Gold oder It_Apple?)."), f[0])
		}
		c, it, am = append(c, who), append(it, f[0]), append(am, strconv.Itoa(n))
	}
	return strings.Join(c, ";"), strings.Join(it, ";"), strings.Join(am, ";"), ""
}
