//go:build windows

package main

// Community Patch pruefen: Ohne ihn (z. B. nach einer frischen Steam-
// Installation, die Gothic 3 in Version 1.60 liefert) passen Vorlagen und
// Navigationskarte nicht zu Mods, die mit dem Patch gespielt werden.
// Eine Mod, die dann gespeichert wird, kann das Spiel abstuerzen lassen.

import (
	"log"
	"os"
	"path/filepath"
)

var noCommunityPatch bool

// checkCommunityPatch warnt, wenn die Archive des Community Patch fehlen.
func (a *app) checkCommunityPatch() {
	data := filepath.Join(a.gothicDir, "Data")
	noCommunityPatch = false
	for _, n := range []string{"Templates.cpt", "Projects_compiled.cpt"} {
		if _, err := os.Stat(filepath.Join(data, n)); err == nil {
			return
		}
	}
	noCommunityPatch = true
	log.Println("Community Patch fehlt (keine .cpt-Archive in Data)")
	messageBox(a.hwnd, T("Der Community Patch 1.75 ist in diesem Gothic-3-Ordner nicht installiert (z. B. nach einer neuen Steam-Installation).\n\nDer Mod Starter braucht ihn, und Mods, die ohne ihn gespeichert werden, passen nicht zum gepatchten Spiel. Speichern ist deshalb gesperrt.\n\nBitte zuerst den Community Patch 1.75.14 installieren und AdanosEdit danach neu starten."), T(appTitleDE), MB_ICONWARNING)
}

// cpSaveBlocked meldet und verhindert das Speichern ohne Community Patch.
func (a *app) cpSaveBlocked() bool {
	if !noCommunityPatch {
		return false
	}
	messageBox(a.hwnd, T("Speichern ist gesperrt: Der Community Patch 1.75 fehlt. Bitte ihn installieren und AdanosEdit neu starten."), T(appTitleDE), MB_ICONWARNING)
	return true
}
