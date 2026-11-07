package main

import (
	"fmt"
	"regexp"

	"g3viewer/lang"
)

func init() {
	for _, p := range lang.FormatProblems() {
		fmt.Println("FORMAT:", p)
	}
	letters := regexp.MustCompile(`[A-Za-zÄÖÜäöüß]{4,}`)
	for _, k := range lang.Keys() {
		for i := 1; i < len(lang.Names); i++ {
			lang.Set(i)
			if v := lang.T(k); v == k && len(letters.FindAllString(k, -1)) > 1 {
				fmt.Printf("GLEICH [%s]: %.80s\n", lang.Code(), k)
			}
		}
	}
	lang.Set(0)
}
