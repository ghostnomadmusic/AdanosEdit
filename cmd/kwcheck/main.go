package main

import (
	"fmt"
	"regexp"
	"strings"

	"g3viewer/lang"
)

var kw = map[string][]string{
	"hero": {"held", "hero", "player", "герой", "bohater"},
	"npc":  {"npc", "нпс", "нип"},
	"act":  {"geben", "give", "дать", "daj", "nehmen", "take", "взять", "weź", "erfahrung", "xp", "experience", "опыт", "doświadczenie", "ereignis", "event", "событие", "zdarzenie", "ende", "end", "конец", "koniec", "warten", "wait", "ждать", "czekaj", "tagesablauf", "routine", "распорядок", "rutyna"},
}

func in(w string, l []string) bool {
	w = strings.ToLower(w)
	for _, x := range l {
		if x == w {
			return true
		}
	}
	return false
}

var german = regexp.MustCompile(`(?i)\b(Tagesablauf|Geben|Nehmen|Erfahrung|Ereignis|Ende|Warten|Vertrieben|Held|Gewählte|Übernehmen|Neuer|Hilfsobjekte|Erst nach|Nicht nach|Bildmitte|Auswahl|Arbeit|Freizeit|Schlaf)\b`)

func main() {
	acts := regexp.MustCompile(`> ([^!\s][^\s“”"«»]*)`)
	for _, k := range lang.Keys() {
		for i := 1; i < len(lang.Names); i++ {
			lang.Set(i)
			v := lang.T(k)
			for _, m := range acts.FindAllStringSubmatch(v, -1) {
				w := strings.TrimRight(m[1], ".,:)")
				if !in(w, kw["act"]) && !strings.HasPrefix(w, "%") {
					fmt.Printf("[%s] Aktion %q in: %.90s\n", lang.Code(), w, strings.ReplaceAll(v, "\n", " "))
				}
			}
			for _, l := range strings.Split(v, "\n") {
				if p, _, ok := strings.Cut(l, ":"); ok && len([]rune(p)) < 12 && !strings.Contains(p, " ") {
					_ = p
				}
			}
			if m := german.FindAllString(v, -1); len(m) > 0 {
				fmt.Printf("[%s] deutsch %v in: %.120s\n", lang.Code(), m, strings.ReplaceAll(v, "\n", " "))
			}
		}
	}
	lang.Set(0)
}
