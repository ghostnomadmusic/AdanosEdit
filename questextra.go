//go:build windows

package main

// Quest-Zusaetze: Questketten (FinishedQuests), Belohnungen ueber Erfahrung
// und Gold hinaus (Ansehen bei einer Gruppe bzw. in einem Ort, Attribut) und
// Zeitlimit (RunningTimeDays) mit Gespraech, wenn es abgelaufen ist.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"g3viewer/g3"
)

// Gruppen (PoliticalSuccess, gEPoliticalAlignment)
var questFactions = []struct {
	v  int
	de string
}{{0, "(keine)"}, {1, "Orks"}, {2, "Nordmarer"}, {3, "Rebellen"}, {4, "Druiden"}, {5, "Hashishin"}, {6, "Nomaden"}}

// Orte (EnclaveSuccess) wie in den Original-Quests
var questEnclaves = []struct{ id, de string }{
	{"", "(keiner)"}, {"Ardea", "Ardea"}, {"Bakaresh", "Bakaresh"}, {"BenErai", "Ben Erai"}, {"BenSala", "Ben Sala"},
	{"Braga", "Braga"}, {"Faring", "Faring"}, {"Fireclan", "Feuerclan"}, {"Geldern", "Geldern"}, {"Gotha", "Gotha"},
	{"Hammerclan", "Hammerclan"}, {"Ishtar", "Ishtar"}, {"KapDun", "Kap Dun"}, {"Enclave_Lago", "Lago"},
	{"Monastery", "Kloster (Nordmar)"}, {"Montera", "Montera"}, {"MoraSul", "Mora Sul"}, {"Nemora", "Nemora"},
	{"Okara", "Okara"}, {"Reddock", "Reddock"}, {"Silden", "Silden"}, {"Trelis", "Trelis"}, {"Vangard", "Vengard"},
	{"Wolfclan", "Wolfsclan"},
}

// Attribute (AttribSuccess)
var questAttribs = []struct{ id, de string }{
	{"", "(keines)"}, {"STR", "Stärke"}, {"DEX", "Geschick"}, {"THF", "Diebeskunst"},
	{"SMT", "Schmiedekunst"}, {"ALC", "Alchemie"}, {"INT", "Altes Wissen"},
}

// Zauber fuer "Zauber auf jemanden wirken" (SpellName) mit Schriftrolle
var questSpells = []struct{ id, scroll, de string }{
	{"Spl_HealOther", "It_Scroll_HealOther", "Heilung anderer"},
	{"Spl_Sleep", "It_Scroll_Sleep", "Schlaf"},
	{"Spl_Fear", "It_Scroll_Fear", "Furcht"},
	{"Spl_Charm", "It_Scroll_Charm", "Bezaubern"},
	{"Spl_Hypnosis", "It_Scroll_Hypnosis", "Hypnose"},
	{"Spl_Telekinesis", "It_Scroll_Telekinesis", "Telekinese"},
	{"Spl_TameAnimal", "It_Scroll_TameAnimal", "Tier zähmen"},
	{"Spl_Poison", "It_Scroll_Poison", "Gift"},
	{"Spl_Disease", "It_Scroll_Disease", "Krankheit"},
	{"Spl_Icelance", "It_Scroll_Icelance", "Eislanze"},
	{"Spl_Lightningbolt", "It_Scroll_Lightningbolt", "Blitz"},
	{"Spl_Fireball", "It_Scroll_Fireball", "Feuerball"},
	{"Spl_DestroyEvil", "It_Scroll_DestroyEvil", "Böses vernichten"},
}

func spellByID(id string) struct{ id, scroll, de string } {
	for _, sp := range questSpells {
		if strings.EqualFold(sp.id, id) {
			return sp
		}
	}
	return questSpells[0]
}

func (a *app) fillRewardCombos() {
	q := &a.qst
	var p, e, at []string
	for _, f := range questFactions {
		p = append(p, T(f.de))
	}
	for _, f := range questEnclaves {
		e = append(e, T(f.de))
	}
	for _, f := range questAttribs {
		at = append(at, T(f.de))
	}
	l := q.loading
	q.loading = true
	comboFill(q.pol, p, max(0, comboSel(q.pol)))
	comboFill(q.enc, e, max(0, comboSel(q.enc)))
	comboFill(q.attr, at, max(0, comboSel(q.attr)))
	var sp []string
	for _, f := range questSpells {
		sp = append(sp, T(f.de))
	}
	comboFill(q.spell, sp, max(0, comboSel(q.spell)))
	q.loading = l
}

func amountText(v int) string {
	if v == 0 {
		return ""
	}
	return strconv.Itoa(v)
}

// showQuestRewards zeigt Belohnungen, Vorbedingung und Zeitlimit.
func (a *app) showQuestRewards(qq *g3.Quest) {
	q := &a.qst
	pi := 0
	for i, f := range questFactions {
		if f.v == qq.GetInt("PoliticalSuccess") {
			pi = i
		}
	}
	sendMessage(q.pol, CB_SETCURSEL, uintptr(pi), 0)
	setText(q.polAmt, amountText(qq.GetInt("PoliticalSuccessAmount")))
	ei := 0
	for i, f := range questEnclaves {
		if f.id != "" && strings.EqualFold(f.id, qq.Get("EnclaveSuccess")) {
			ei = i
		}
	}
	sendMessage(q.enc, CB_SETCURSEL, uintptr(ei), 0)
	setText(q.encAmt, amountText(qq.GetInt("EnclaveSuccessAmount")))
	ai := 0
	for i, f := range questAttribs {
		if f.id != "" && strings.EqualFold(f.id, qq.Get("AttribSuccess")) {
			ai = i
		}
	}
	sendMessage(q.attr, CB_SETCURSEL, uintptr(ai), 0)
	setText(q.attrAmt, amountText(qq.GetInt("AttribSuccessAmount")))
	setText(q.prereq, strings.ReplaceAll(qq.Get("FinishedQuests"), ";", "; "))
	setText(q.time, amountText(qq.GetInt("RunningTimeDays")))
}

// applyQuestRewards uebernimmt die Felder in die Quest; Rueckgabe = Fehler.
func (a *app) applyQuestRewards(nq *g3.Quest) string {
	q := &a.qst
	num := func(h uintptr, lo, hi int) (int, string) {
		s := strings.TrimSpace(getText(h))
		if s == "" {
			return 0, ""
		}
		v, err := strconv.Atoi(s)
		if err != nil || v < lo || v > hi {
			return 0, fmt.Sprintf(T("„%s“ ist keine gültige Zahl (erlaubt: %d bis %d)."), s, lo, hi)
		}
		return v, ""
	}
	pol, msg := num(q.polAmt, -100, 100)
	if msg != "" {
		return msg
	}
	enc, msg := num(q.encAmt, -100, 100)
	if msg != "" {
		return msg
	}
	att, msg := num(q.attrAmt, 0, 100)
	if msg != "" {
		return msg
	}
	days, msg := num(q.time, 0, 365)
	if msg != "" {
		return msg
	}
	pi, ei, ai := max(0, comboSel(q.pol)), max(0, comboSel(q.enc)), max(0, comboSel(q.attr))
	if pi > 0 && pol == 0 {
		return T("Bitte beim Ansehen der Gruppe auch einen Betrag eintragen (z. B. 5).")
	}
	if ei > 0 && enc == 0 {
		return T("Bitte beim Ansehen im Ort auch einen Betrag eintragen (z. B. 5).")
	}
	if ai > 0 && att == 0 {
		return T("Bitte beim Attribut auch einen Betrag eintragen (z. B. 1).")
	}
	if pi == 0 {
		pol = 0
	}
	if ei == 0 {
		enc = 0
	}
	if ai == 0 {
		att = 0
	}
	nq.SetInt("PoliticalSuccess", questFactions[pi].v)
	nq.SetInt("PoliticalSuccessAmount", pol)
	nq.Set("EnclaveSuccess", questEnclaves[ei].id)
	nq.SetInt("EnclaveSuccessAmount", enc)
	nq.Set("AttribSuccess", questAttribs[ai].id)
	nq.SetInt("AttribSuccessAmount", att)
	nq.SetInt("RunningTimeDays", days)
	// Vorbedingung: Quest-Namen (auch Titel eigener Quests werden erkannt)
	var pre []string
	for _, t := range strings.FieldsFunc(getText(q.prereq), func(r rune) bool { return r == ';' || r == ',' }) {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		name := a.questNameFor(t)
		if name == "" {
			return fmt.Sprintf(T("Eine Quest „%s“ gibt es nicht. Bitte den Namen aus der Vorschlagsliste wählen (z. B. G3V_Quest_01)."), t)
		}
		if strings.EqualFold(name, nq.Name()) {
			return T("Eine Quest kann nicht auf sich selbst warten.")
		}
		if a.questChainHas(name, nq.Name(), 0) {
			return fmt.Sprintf(T("„%s“ wartet selbst schon auf diese Quest – das wäre ein Kreis, keine der beiden könnte je beginnen."), name)
		}
		pre = append(pre, name)
	}
	nq.Set("FinishedQuests", strings.Join(pre, ";"))
	return ""
}

// questNameFor findet eine Quest ueber Namen oder Titel ("" = unbekannt).
func (a *app) questNameFor(s string) string {
	if qq := a.questByName(s); qq != nil {
		return qq.Name()
	}
	for _, qq := range a.qst.quests {
		if strings.EqualFold(a.questTitle(qq.Name()), s) {
			return qq.Name()
		}
	}
	return ""
}

// questChainHas: wartet die Quest from (direkt oder ueber andere) auf target?
func (a *app) questChainHas(from, target string, depth int) bool {
	if depth > 50 {
		return true
	}
	qq := a.questByName(from)
	if qq == nil {
		return false
	}
	for _, p := range strings.Split(qq.Get("FinishedQuests"), ";") {
		if p == "" {
			continue
		}
		if strings.EqualFold(p, target) || a.questChainHas(p, target, depth+1) {
			return true
		}
	}
	return false
}

// questItems liefert alle Quests fuer die Vorschlagsliste "Erst nach Quest"
// (eigene zuerst, mit Titel).
func (a *app) questItems() []acItem {
	a.loadQuests()
	var out []acItem
	for p, qq := range a.qst.quests {
		n := qq.Name()
		if n == "" {
			continue
		}
		label := n
		if t := a.questTitle(n); t != n {
			label = n + " – " + t
		}
		prio := 1
		if strings.HasPrefix(p, "g3_world_01/g3v_") || strings.HasPrefix(strings.ToLower(n), "g3v_") {
			prio = 0
		}
		out = append(out, acItem{name: n, label: label, lower: strings.ToLower(n), labelLower: strings.ToLower(label), prio: prio})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].lower < out[j].lower })
	return out
}

const overtimeText = "NPC: Du hast zu lange gebraucht. Der Auftrag hat sich erledigt."

// overtimeScript: Gespraech, wenn das Zeitlimit abgelaufen ist (der
// Auftraggeber spricht den Helden an und beendet die Quest).
func overtimeScript(name string) string {
	return T(overtimeText) + "\n> !CloseQuest;;;" + name
}

// updateOvertimeDialog legt das Gespraech fuer ein abgelaufenes Zeitlimit an
// (wenn die Quest schon Dialoge hat) oder entfernt es ohne Zeitlimit.
func (a *app) updateOvertimeDialog(nq *g3.Quest, giver string) {
	a.loadDialogs()
	name := nq.Name()
	var own []string
	var overtime []string
	for lp, in := range a.dlg.infos {
		if !isOwnInfo(in) || !strings.EqualFold(in.Quest, name) {
			continue
		}
		own = append(own, lp)
		if in.ConditionType == g3.CondOvertime {
			overtime = append(overtime, lp)
		}
	}
	days := nq.GetInt("RunningTimeDays")
	if days == 0 {
		for _, lp := range overtime {
			p := a.dlg.paths[lp]
			if p == "" {
				p = lp
			}
			a.ed.setExtra("Infos", p, nil)
			delete(a.dlg.infos, lp)
		}
		if len(overtime) > 0 {
			a.indexDialogs()
		}
		return
	}
	if len(overtime) > 0 || len(own) == 0 {
		return // schon da, oder die Dialoge werden noch erzeugt
	}
	a.addOwnInfo(giver, g3.CondOvertime, g3.InfoImportant, name, false, overtimeScript(name))
	a.indexDialogs()
}

// enclaveLabel: lesbarer Name einer Enklave (bekannte Orte uebersetzt).
func enclaveLabel(id string) string {
	for _, f := range questEnclaves {
		if f.id != "" && strings.EqualFold(f.id, id) {
			return T(f.de)
		}
	}
	return id
}

// enclaveItems liefert alle Enklaven der Welt fuer die Vorschlagsliste.
func (a *app) enclaveItems() []acItem {
	if a.world == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []acItem
	for i := range a.world.Inst {
		e := a.world.Inst[i].Ent
		if a.world.Inst[i].Hidden || e.Name == "" || !hasClass(e, "gCEnclave_PS") || seen[strings.ToLower(e.Name)] {
			continue
		}
		seen[strings.ToLower(e.Name)] = true
		label := e.Name
		if l := enclaveLabel(e.Name); l != e.Name {
			label = e.Name + " – " + l
		}
		out = append(out, acItem{name: e.Name, label: label, lower: strings.ToLower(e.Name), labelLower: strings.ToLower(label), prio: 1})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].lower < out[j].lower })
	return out
}

// enclaveMembers zaehlt die (nicht geloeschten) NPCs einer Enklave.
// ok = false: es gibt keine Enklave dieses Namens.
func (a *app) enclaveMembers(name string) (n int, ok bool) {
	if a.world == nil {
		return 0, true
	}
	var g [16]byte
	for i := range a.world.Inst {
		e := a.world.Inst[i].Ent
		if !a.world.Inst[i].Hidden && strings.EqualFold(e.Name, name) && hasClass(e, "gCEnclave_PS") {
			g, ok = e.GUID, true
			break
		}
	}
	if !ok {
		return 0, false
	}
	for i := range a.world.Inst {
		in := &a.world.Inst[i]
		if in.Hidden || in.Ent.Kind != g3.KindNPC || !hasClass(in.Ent, "gCNPC_PS") {
			continue
		}
		wf, err := a.editFile(in)
		if err != nil {
			continue
		}
		if enc, _, err := wf.Faction(in.Ent.Index); err == nil && enc == g {
			n++
		}
	}
	return n, true
}
