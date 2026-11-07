//go:build windows

package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"g3viewer/g3"
)

// runExtract ist eine Diagnosehilfe: Liegt AdanosEdit_extract.txt neben dem
// Programm, werden die darin genannten Dateien nach extract\ entpackt.
// Zeilen: "list" (Inhaltsverzeichnis aller Archive) oder "Archiv/Teilname".
func runExtract(v *g3.VFS) {
	cmdFile := filepath.Join(exeDir(), "AdanosEdit_extract.txt")
	b, err := os.ReadFile(cmdFile)
	if err != nil {
		return
	}
	outDir := filepath.Join(exeDir(), "extract")
	os.MkdirAll(outDir, 0755)
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == "list" {
			f, err := os.Create(filepath.Join(outDir, "files.txt"))
			if err != nil {
				continue
			}
			w := bufio.NewWriter(f)
			for _, a := range v.Archives {
				cat := strings.TrimSuffix(filepath.Base(a.Path), filepath.Ext(a.Path))
				for _, e := range a.Entries {
					fmt.Fprintf(w, "%s\t%s\t%d\n", cat, e.Name, e.Size)
				}
			}
			w.Flush()
			f.Close()
			continue
		}
		i := strings.Index(line, "/")
		if i < 0 {
			continue
		}
		cat, sub := line[:i], strings.ToLower(line[i+1:])
		n := 0
		for _, e := range v.List(cat) {
			if !strings.Contains(strings.ToLower(e.Name), sub) {
				continue
			}
			d, err := e.Read()
			if err != nil {
				continue
			}
			name := strings.ReplaceAll(e.Name, "/", "__")
			os.MkdirAll(filepath.Join(outDir, cat), 0755)
			os.WriteFile(filepath.Join(outDir, cat, name), d, 0644)
			n++
			if n >= 300 {
				break
			}
		}
		log.Printf("Export %s: %d Dateien", line, n)
	}
	os.Rename(cmdFile, cmdFile+".erledigt")
}
