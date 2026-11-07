package main

// Umwandlung eines Gespraechsablaufs (.info) in eine einfache Textform und
// zurueck (fuer den Dialog-Reiter).

import (
	"fmt"
	"strconv"
	"strings"

	"g3viewer/g3"
	"g3viewer/lang"
)

type scriptCtx struct {
	text   func(key string) (string, bool) // Text eines Schluessels
	exists func(key string) bool           // Schluessel schon vergeben?
	items  map[string]bool                 // bekannte Gegenstaende (klein), nil = nicht pruefen
}

var tr = lang.T

// --- Text <-> Ablauf -------------------------------------------------------

var (
	heroWords = []string{"held", "hero", "player", "герой", "bohater"}
	npcWords  = []string{"npc", "нпс", "нип"}
	actGive   = []string{"geben", "give", "дать", "daj"}
	actTake   = []string{"nehmen", "take", "взять", "weź"}
	actXP     = []string{"erfahrung", "xp", "experience", "опыт", "doświadczenie"}
	actEvent  = []string{"ereignis", "event", "событие", "zdarzenie"}
	actEnd    = []string{"ende", "end", "конец", "koniec"}
	actWait   = []string{"warten", "wait", "ждать", "czekaj"}
	actRout   = []string{"tagesablauf", "routine", "распорядок", "rutyna"}
	actTrade  = []string{"handel", "trade", "торговля"}
	actTeach  = []string{"lehren", "teach", "обучение", "nauka"}
	wordOn    = []string{"an", "ein", "on", "вкл", "wł", "wl"}
	wordOff   = []string{"aus", "off", "выкл", "wył", "wyl"}
)

func inWords(w string, l []string) bool {
	w = strings.ToLower(w)
	for _, x := range l {
		if w == x {
			return true
		}
	}
	return false
}

// scriptText wandelt den Ablauf in die editierbare Textform.
func scriptText(in *g3.Info, ctx scriptCtx) string {
	var lines []string
	sc := in.Script
	for i := 0; i < len(sc); i++ {
		c := sc[i]
		switch {
		case c.Cmd == "SetGameEvent" && c.ID1 == in.Name && strings.HasPrefix(in.Name, "G3V_"):
			// Markierung fuer "Erst nach" (wird automatisch gesetzt)
		case c.Cmd == "Say" && (c.Ent1 == "player" || c.Ent1 == "npc") && c.Ent2 == "" && c.ID1 == "":
			who := tr("NPC")
			if c.Ent1 == "player" {
				who = tr("Held")
			}
			if c.ID2 != "" {
				who += " (" + c.ID2 + ")"
			}
			t, ok := ctx.text(c.Text)
			if !ok {
				t = "[" + c.Text + "]"
			}
			lines = append(lines, who+": "+t)
		case c.Cmd == "Grant" && c.Ent1 == "npc" && i+1 < len(sc) && sc[i+1].Cmd == "Give" && sc[i+1].Ent1 == "npc" &&
			sc[i+1].Ent2 == "player" && sc[i+1].ID1 == c.ID1 && sc[i+1].ID2 == c.ID2:
			lines = append(lines, "> "+tr("Geben")+" "+c.ID1+" "+c.ID2)
			i++
		case c.Cmd == "Give" && c.Ent1 == "npc" && c.Ent2 == "player":
			lines = append(lines, "> "+tr("Geben")+" "+c.ID1+" "+c.ID2)
		case c.Cmd == "Give" && c.Ent1 == "player" && c.Ent2 == "npc":
			lines = append(lines, "> "+tr("Nehmen")+" "+c.ID1+" "+c.ID2)
		case c.Cmd == "GiveXP":
			n, err := strconv.Atoi(c.ID1)
			if err != nil {
				lines = append(lines, rawCmd(c))
				continue
			}
			lines = append(lines, "> "+tr("Erfahrung")+" "+strconv.Itoa(n*5))
		case c.Cmd == "SetGameEvent" && c.Ent2 == "" && c.ID2 == "":
			lines = append(lines, "> "+tr("Ereignis")+" "+c.ID1)
		case c.Cmd == "End" && c.Ent1 == "" && c.ID1 == "":
			lines = append(lines, "> "+tr("Ende"))
		case c.Cmd == "SetRoutine" && c.Ent2 == "" && c.ID2 == "" && c.Text == "" && c.ID1 != "" && !strings.ContainsAny(c.Ent1+c.ID1, " ;"):
			who := c.Ent1
			if who == "" {
				who = "npc"
			}
			lines = append(lines, "> "+tr("Tagesablauf")+" "+who+" "+c.ID1)
		case (c.Cmd == "SetTradeEnabled" || c.Cmd == "SetTeachEnabled") && c.Ent2 == "" && c.ID2 == "" && c.Text == "" &&
			(c.ID1 == "true" || c.ID1 == "false") && !strings.ContainsAny(c.Ent1, " ;"):
			l := "> " + tr("Handel")
			if c.Cmd == "SetTeachEnabled" {
				l = "> " + tr("Lehren")
			}
			if c.ID1 == "true" {
				l += " " + tr("an")
			} else {
				l += " " + tr("aus")
			}
			if c.Ent1 != "" && c.Ent1 != "npc" {
				l += " " + c.Ent1
			}
			lines = append(lines, l)
		case c.Cmd == "InfoWait":
			lines = append(lines, "> "+tr("Warten")+" "+c.ID1)
		default:
			lines = append(lines, rawCmd(c))
		}
	}
	return strings.Join(lines, "\r\n")
}

func rawCmd(c g3.InfoCmd) string {
	return "> !" + strings.Join([]string{c.Cmd, c.Ent1, c.Ent2, c.ID1, c.ID2, c.Text}, ";")
}

// parseScript wandelt die Textform in einen Ablauf. Neue oder geaenderte
// Saetze bekommen neue Schluessel (Texte), unveraenderte behalten ihren
// (damit bleibt auch die Sprachaufnahme erhalten).
func parseScript(text string, in *g3.Info, ctx scriptCtx) (script []g3.InfoCmd, newStrs map[string]string, warn []string, err error) {
	newStrs = map[string]string{}
	// vorhandene Saetze: Text -> Schluessel
	old := map[string][]string{}
	for _, c := range in.Script {
		if c.Cmd == "Say" && c.Text != "" {
			if t, ok := ctx.text(c.Text); ok {
				k := c.Ent1 + "|" + strings.TrimSpace(t)
				old[k] = append(old[k], c.Text)
			}
		}
	}
	used := map[string]bool{}
	next := 0
	newKey := func() string {
		for {
			k := fmt.Sprintf("INFO_%s_%d", in.Name, next)
			next++
			if ctx.exists(k) || used[k] {
				continue
			}
			if _, ok := newStrs[k]; ok {
				continue
			}
			return k
		}
	}
	items := ctx.items
	for n, line := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		bad := func(msg string) error {
			return fmt.Errorf(tr("Zeile %d: %s"), n+1, msg)
		}
		if strings.HasPrefix(line, ">") {
			act := strings.TrimSpace(line[1:])
			if strings.HasPrefix(act, "!") {
				f := strings.Split(act[1:], ";")
				for len(f) < 6 {
					f = append(f, "")
				}
				script = append(script, g3.InfoCmd{Cmd: strings.TrimSpace(f[0]), Ent1: f[1], Ent2: f[2], ID1: f[3], ID2: f[4], Text: f[5]})
				continue
			}
			w := strings.Fields(act)
			if len(w) == 0 {
				return nil, nil, nil, bad(tr("Aktion fehlt."))
			}
			num := func(i int, def int) (int, error) {
				if len(w) <= i {
					return def, nil
				}
				v, e := strconv.Atoi(w[i])
				if e != nil || v < 0 {
					return 0, bad(fmt.Sprintf(tr("„%s“ ist keine gültige Zahl."), w[i]))
				}
				return v, nil
			}
			switch {
			case inWords(w[0], actGive), inWords(w[0], actTake):
				if len(w) < 2 {
					return nil, nil, nil, bad(tr("Welcher Gegenstand? Beispiel: > Geben It_Gold 50"))
				}
				cnt, e := num(2, 1)
				if e != nil {
					return nil, nil, nil, e
				}
				it := w[1]
				if items != nil && !items[strings.ToLower(it)] {
					warn = append(warn, fmt.Sprintf(tr("Zeile %d: Den Gegenstand „%s“ kenne ich nicht (Vorlagenname wie It_Gold oder It_Apple?)."), n+1, it))
				}
				c := strconv.Itoa(cnt)
				if inWords(w[0], actGive) {
					script = append(script, g3.InfoCmd{Cmd: "Grant", Ent1: "npc", ID1: it, ID2: c},
						g3.InfoCmd{Cmd: "Give", Ent1: "npc", Ent2: "player", ID1: it, ID2: c})
				} else {
					script = append(script, g3.InfoCmd{Cmd: "Give", Ent1: "player", Ent2: "npc", ID1: it, ID2: c})
				}
			case inWords(w[0], actXP):
				v, e := num(1, -1)
				if e != nil || v < 0 {
					if e == nil {
						e = bad(tr("Wie viel? Beispiel: > Erfahrung 100"))
					}
					return nil, nil, nil, e
				}
				x := (v + 2) / 5
				if x < 1 {
					x = 1
				}
				script = append(script, g3.InfoCmd{Cmd: "GiveXP", ID1: strconv.Itoa(x)})
			case inWords(w[0], actEvent):
				if len(w) < 2 {
					return nil, nil, nil, bad(tr("Name des Ereignisses fehlt."))
				}
				script = append(script, g3.InfoCmd{Cmd: "SetGameEvent", ID1: w[1]})
			case inWords(w[0], actRout):
				if len(w) < 2 {
					return nil, nil, nil, bad(tr("Welcher Tagesablauf? Beispiel: > Tagesablauf Felix BeiJohn"))
				}
				who, name := "npc", w[1]
				if len(w) >= 3 {
					who, name = w[1], w[2]
					if inWords(who, npcWords) {
						who = "npc"
					}
				}
				script = append(script, g3.InfoCmd{Cmd: "SetRoutine", Ent1: who, ID1: name})
			case inWords(w[0], actEnd):
				script = append(script, g3.InfoCmd{Cmd: "End"})
			case inWords(w[0], actTrade), inWords(w[0], actTeach):
				cmd := "SetTradeEnabled"
				if inWords(w[0], actTeach) {
					cmd = "SetTeachEnabled"
				}
				if len(w) < 2 || !(inWords(w[1], wordOn) || inWords(w[1], wordOff)) {
					return nil, nil, nil, bad(tr("An oder aus? Beispiel: > Handel an"))
				}
				on := "true"
				if inWords(w[1], wordOff) {
					on = "false"
				}
				who := "npc"
				if len(w) >= 3 && !inWords(w[2], npcWords) {
					who = w[2]
				}
				script = append(script, g3.InfoCmd{Cmd: cmd, Ent1: who, ID1: on})
			case inWords(w[0], actWait):
				v, e := num(1, 1000)
				if e != nil {
					return nil, nil, nil, e
				}
				script = append(script, g3.InfoCmd{Cmd: "InfoWait", ID1: strconv.Itoa(v)})
			default:
				return nil, nil, nil, bad(fmt.Sprintf(tr("Unbekannte Aktion „%s“."), w[0]))
			}
			continue
		}
		// Satz: "Held: Text" / "NPC (SD_Angry): Text"
		i := strings.Index(line, ":")
		if i <= 0 {
			return nil, nil, nil, bad(tr("Wer spricht? Zeilen beginnen mit „Held:“ oder „NPC:“, Aktionen mit „>“."))
		}
		who := strings.TrimSpace(line[:i])
		say := strings.TrimSpace(line[i+1:])
		gest := ""
		if j := strings.Index(who, "("); j >= 0 && strings.HasSuffix(who, ")") {
			gest = strings.TrimSpace(who[j+1 : len(who)-1])
			who = strings.TrimSpace(who[:j])
		}
		ent := ""
		switch {
		case inWords(who, heroWords) || strings.EqualFold(who, tr("Held")):
			ent = "player"
		case inWords(who, npcWords) || strings.EqualFold(who, tr("NPC")) || strings.EqualFold(who, in.Owner):
			ent = "npc"
		default:
			return nil, nil, nil, bad(fmt.Sprintf(tr("„%s“ ist kein Sprecher. Zeilen beginnen mit „Held:“ oder „NPC:“."), who))
		}
		if say == "" {
			return nil, nil, nil, bad(tr("Der Satz ist leer."))
		}
		var key string
		if strings.HasPrefix(say, "[") && strings.HasSuffix(say, "]") && !strings.ContainsAny(say, " \t") && len(say) > 2 {
			key = say[1 : len(say)-1]
		} else if ks := old[ent+"|"+say]; len(ks) > 0 {
			key = ks[0]
			old[ent+"|"+say] = ks[1:]
		} else {
			key = newKey()
			newStrs[key] = say
		}
		used[key] = true
		script = append(script, g3.InfoCmd{Cmd: "Say", Ent1: ent, ID2: gest, Text: key})
	}
	return script, newStrs, warn, nil
}
