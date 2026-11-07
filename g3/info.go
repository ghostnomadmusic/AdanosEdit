package g3

// Dialoge (.info) und Quests (.quest) von Gothic 3. Beide sind einfache
// INI-Textdateien in Infos.pak bzw. Quests.pak (G3_World_01/...).

import (
	"strconv"
	"strings"
)

// Info-Arten (InfoType)
const (
	InfoRefuse    = 0
	InfoImportant = 1 // NPC spricht den Helden von selbst an
	InfoNews      = 2
	InfoInfo      = 3 // normale Auswahl im Gespraech
	InfoParent    = 4 // Thema: oeffnet eine Unterauswahl (Kind-Dialoge mit Parent)
	InfoComment   = 5
	InfoPush      = 7
)

// Bedingungsarten (ConditionType), Auswahl
const (
	CondHello   = 2
	CondGeneral = 3
	CondRunning = 7
	CondSuccess = 10
	CondTrade   = 17
)

// InfoCmd ist ein Kommando im Ablauf eines Gespraechs (InfoScript_*).
type InfoCmd struct {
	Cmd  string // Say, Give, GiveXP, RunQuest ...
	Ent1 string // z. B. player, npc
	Ent2 string
	ID1  string
	ID2  string // bei Say: Geste (StageDirection)
	Text string // bei Say: Schluessel in der stringtable.ini (INFO_...)
}

// Info ist ein Gespraechseintrag eines NPCs.
type Info struct {
	Name               string
	SortID             int
	Owner              string
	Parent             string
	Quest              string
	ConditionType      int
	InfoType           int
	GoldCost           int
	Folder             string
	CondPlayerKnows    string
	CondPlayerKnowsNot string // Dialog verschwindet, sobald eines dieser Ereignisse gesetzt ist
	hadNot             bool
	// Zustand weiterer NPCs (gEInfoNPCStatus je NPC, ";"-getrennt):
	// 0 lebt, 1 unversehrt (nicht besiegt), 2 vom Helden besiegt, 3 tot,
	// 4 hat mit dem Helden gesprochen, 5 noch nicht
	CondSecondaryNPC       string
	CondSecondaryNPCstates string
	// Gegenstaende, die jemand haben muss (je Eintrag Behaelter, Vorlage, Anzahl)
	CondItemContainer string
	CondItems         string
	CondItemAmounts   string
	ClearChildren     bool
	Permanent         bool
	Script            []InfoCmd

	// Zeilen der gelesenen Datei (damit unbekannte Felder erhalten bleiben)
	raw []string
}

// InfoPath ist der Pfad der Info-Datei im Archiv Infos.
func InfoPath(name string) string {
	return "G3_World_01/" + name + "_info_G3_World_01.info"
}

// Bytes erzeugt den Dateiinhalt (wie die Originaldateien, CRLF).
func (in *Info) Bytes() []byte {
	fresh := in.bytesFresh()
	if in.raw == nil {
		return fresh
	}
	return mergeInfoLines(in.raw, fresh)
}

func (in *Info) bytesFresh() []byte {
	var b strings.Builder
	kv := func(k, v string) {
		if v == "" {
			b.WriteString(k + "\r\n")
		} else {
			b.WriteString(k + "=" + v + "\r\n")
		}
	}
	bl := func(v bool) string { return strconv.FormatBool(v) }
	b.WriteString("[Info]\r\n")
	kv("Name", in.Name)
	kv("SortID", strconv.Itoa(in.SortID))
	kv("Owner", in.Owner)
	kv("InfoGiven", "false")
	kv("Parent", in.Parent)
	kv("Quest", in.Quest)
	kv("ConditionType", strconv.Itoa(in.ConditionType))
	kv("InfoType", strconv.Itoa(in.InfoType))
	kv("GoldCost", strconv.Itoa(in.GoldCost))
	kv("CondOwnerNearEntity", "")
	kv("Folder", in.Folder)
	kv("CondPlayerKnows", in.CondPlayerKnows)
	if in.CondPlayerKnowsNot != "" || in.hadNot {
		kv("CondPlayerKnowsNot", in.CondPlayerKnowsNot)
	}
	kv("CondItemContainer", in.CondItemContainer)
	kv("CondItems", in.CondItems)
	kv("CondItemAmounts", in.CondItemAmounts)
	kv("CondSecondaryNPC", in.CondSecondaryNPC)
	kv("CondSecondaryNPCstates", in.CondSecondaryNPCstates)
	kv("ClearChildren", bl(in.ClearChildren))
	kv("Permanent", bl(in.Permanent))
	col := func(f func(c InfoCmd) string) string {
		s := make([]string, len(in.Script))
		for i, c := range in.Script {
			s[i] = f(c)
		}
		return strings.Join(s, ";")
	}
	kv("InfoScript_Commands", col(func(c InfoCmd) string { return c.Cmd }))
	kv("InfoScript_Entities1", col(func(c InfoCmd) string { return c.Ent1 }))
	kv("InfoScript_Entities2", col(func(c InfoCmd) string { return c.Ent2 }))
	kv("InfoScript_IDs1", col(func(c InfoCmd) string { return c.ID1 }))
	kv("InfoScript_IDs2", col(func(c InfoCmd) string { return c.ID2 }))
	kv("InfoScript_Texts", col(func(c InfoCmd) string { return c.Text }))
	return []byte(b.String())
}

// ParseInfo liest eine Info-Datei (nur die hier verwendeten Felder).
func ParseInfo(d []byte) *Info {
	in := &Info{}
	for _, l := range strings.Split(string(d), "\n") {
		if l = strings.TrimRight(l, "\r"); l != "" {
			in.raw = append(in.raw, l)
		}
	}
	cols := map[string][]string{}
	for _, line := range strings.Split(string(d), "\n") {
		line = strings.TrimRight(line, "\r")
		k, v, _ := strings.Cut(line, "=")
		switch k {
		case "Name":
			in.Name = v
		case "SortID":
			in.SortID, _ = strconv.Atoi(v)
		case "Owner":
			in.Owner = v
		case "Parent":
			in.Parent = v
		case "Quest":
			in.Quest = v
		case "ConditionType":
			in.ConditionType, _ = strconv.Atoi(v)
		case "InfoType":
			in.InfoType, _ = strconv.Atoi(v)
		case "GoldCost":
			in.GoldCost, _ = strconv.Atoi(v)
		case "Folder":
			in.Folder = v
		case "CondPlayerKnows":
			in.CondPlayerKnows = v
		case "CondPlayerKnowsNot":
			in.CondPlayerKnowsNot = v
			in.hadNot = true
		case "CondItemContainer":
			in.CondItemContainer = v
		case "CondItems":
			in.CondItems = v
		case "CondItemAmounts":
			in.CondItemAmounts = v
		case "CondSecondaryNPC":
			in.CondSecondaryNPC = v
		case "CondSecondaryNPCstates":
			in.CondSecondaryNPCstates = v
		case "ClearChildren":
			in.ClearChildren = v == "true"
		case "Permanent":
			in.Permanent = v == "true"
		default:
			if strings.HasPrefix(k, "InfoScript_") {
				cols[k] = strings.Split(v, ";")
			}
		}
	}
	n := len(cols["InfoScript_Commands"])
	if n == 1 && cols["InfoScript_Commands"][0] == "" {
		n = 0
	}
	get := func(k string, i int) string {
		if c := cols[k]; i < len(c) {
			return c[i]
		}
		return ""
	}
	for i := 0; i < n; i++ {
		in.Script = append(in.Script, InfoCmd{
			Cmd: get("InfoScript_Commands", i), Ent1: get("InfoScript_Entities1", i),
			Ent2: get("InfoScript_Entities2", i), ID1: get("InfoScript_IDs1", i),
			ID2: get("InfoScript_IDs2", i), Text: get("InfoScript_Texts", i),
		})
	}
	return in
}

// mergeInfoLines schreibt die neuen Werte in die Zeilen der Originaldatei:
// bekannte Felder werden ersetzt (in ihrer Reihenfolge), fehlende ergaenzt,
// alle anderen Zeilen bleiben unveraendert.
func mergeInfoLines(raw []string, fresh []byte) []byte {
	key := func(l string) string {
		k, _, _ := strings.Cut(l, "=")
		return k
	}
	nl := map[string]string{}
	var order []string
	keepRaw := map[string]bool{"InfoGiven": true, "CondOwnerNearEntity": true}
	have := map[string]bool{}
	for _, l := range raw {
		have[key(l)] = true
	}
	for _, l := range strings.Split(string(fresh), "\r\n") {
		if l == "" || l == "[Info]" || (keepRaw[key(l)] && have[key(l)]) {
			continue
		}
		nl[key(l)] = l
		order = append(order, key(l))
	}
	done := map[string]bool{}
	var out []string
	for _, l := range raw {
		k := key(l)
		if n, ok := nl[k]; ok && !done[k] {
			_, v1, _ := strings.Cut(l, "=")
			_, v2, _ := strings.Cut(n, "=")
			if v1 == v2 {
				n = l // gleicher Wert: Originalschreibweise behalten
			}
			out = append(out, n)
			done[k] = true
			continue
		}
		if _, ok := nl[k]; ok {
			continue // doppelt
		}
		out = append(out, l)
	}
	for _, k := range order {
		if !done[k] {
			out = append(out, nl[k])
		}
	}
	return []byte(strings.Join(out, "\r\n") + "\r\n")
}
