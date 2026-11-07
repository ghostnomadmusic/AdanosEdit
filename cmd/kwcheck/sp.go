package main

import (
	"fmt"
	"strings"

	"g3viewer/lang"
)

func init() {
	n := 0
	for _, k := range lang.Keys() {
		var want []string
		if strings.HasPrefix(k, "Held: ") {
			want = kw["hero"]
		} else if strings.HasPrefix(k, "NPC: ") {
			want = kw["npc"]
		} else {
			continue
		}
		n++
		for i := 1; i < len(lang.Names); i++ {
			lang.Set(i)
			v := lang.T(k)
			p, _, ok := strings.Cut(v, ":")
			if !ok || !in(strings.TrimSpace(p), want) {
				fmt.Printf("[%s] Sprecher falsch: %.80s\n", lang.Code(), v)
			}
		}
	}
	lang.Set(0)
	fmt.Println("Sprecherzeilen geprüft:", n)
}
