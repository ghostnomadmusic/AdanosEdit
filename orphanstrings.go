//go:build windows

package main

// Herrenlose Text-Archive: Liegt im Data-Ordner eine Strings.mod (bzw.
// .m00 ...), die der Mod Starter nicht in seiner Liste (fdm.dat) fuehrt,
// entfernt "Delete mods from game" sie nicht. Sie bleibt dann immer aktiv,
// und der Mod Starter baut beim Integrieren jeder Mod seine Texte darauf auf -
// so tauchen Texte alter Mods (z. B. ein geaenderter Heldenname) ueberall auf.

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func (a *app) checkOrphanStrings() {
	mds := a.findMDS()
	if mds == "" || loadConfig()["SaveMode"] == "data" {
		return
	}
	b, err := os.ReadFile(filepath.Join(mds, "fdm.dat"))
	if err != nil {
		return // ohne Liste lieber nichts anfassen
	}
	managed := map[string]bool{}
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			managed[strings.ToLower(filepath.Base(l))] = true
		}
	}
	data := filepath.Join(a.gothicDir, "Data")
	ents, err := os.ReadDir(data)
	if err != nil {
		return
	}
	re := regexp.MustCompile(`(?i)^strings\.(mod|m\d\d)$`)
	var orphans []string
	for _, e := range ents {
		if !e.IsDir() && re.MatchString(e.Name()) && !managed[strings.ToLower(e.Name())] {
			orphans = append(orphans, e.Name())
		}
	}
	if len(orphans) == 0 {
		return
	}
	msg := fmt.Sprintf(T("Im Data-Ordner liegen Text-Archive, die der Mod Starter nicht verwaltet:\n%s\n\nSie stammen meist von einer früheren Integration und bleiben immer aktiv – dadurch tauchen Texte alter Mods (z. B. Namen) auch in anderen Mods auf. Die Originaltexte stecken in Strings.pak.\n\nJetzt abschalten (umbenennen in „.alt“)? Danach im Mod Starter die Mods einmal neu integrieren."), strings.Join(orphans, ", "))
	if messageBoxRet(a.hwnd, msg, T(appTitleDE), MB_YESNO|MB_ICONWARNING) != IDYES {
		return
	}
	for _, n := range orphans {
		p := filepath.Join(data, n)
		if err := os.Rename(p, p+".alt"); err != nil {
			messageBox(a.hwnd, fmt.Sprintf(T("%s konnte nicht umbenannt werden (%s).\nBitte von Hand umbenennen (eventuell mit Administratorrechten)."), n, err.Error()), T(appTitleDE), MB_ICONERROR)
			continue
		}
		log.Println("Herrenloses Text-Archiv abgeschaltet:", n)
	}
}
