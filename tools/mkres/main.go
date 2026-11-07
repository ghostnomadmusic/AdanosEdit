// mkres erzeugt rsrc_windows_amd64.syso (Programmsymbol und Dateiinformationen).
//
// Aufruf (im Ordner tools/mkres):
//   go run . 1.3.0.0 "1.3" ../../rsrc_windows_amd64.syso ../../assets/icon
package main

import (
	"image"
	_ "image/png"
	"os"
	"path/filepath"

	"github.com/tc-hib/winres"
	"github.com/tc-hib/winres/version"
)

func main() {
	var imgs []image.Image
	for _, s := range []string{"16", "24", "32", "48", "64", "128", "256"} {
		f, err := os.Open(filepath.Join(os.Args[4], "icon_"+s+".png"))
		if err != nil {
			panic(err)
		}
		img, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			panic(err)
		}
		imgs = append(imgs, img)
	}
	icon, err := winres.NewIconFromImages(imgs)
	if err != nil {
		panic(err)
	}
	rs := winres.ResourceSet{}
	if err := rs.SetIcon(winres.Name("APP"), icon); err != nil {
		panic(err)
	}
	vi := version.Info{}
	vi.SetFileVersion(os.Args[1])
	vi.SetProductVersion(os.Args[1])
	for k, v := range map[string]string{
		version.ProductName:      "AdanosEdit",
		version.FileDescription:  "AdanosEdit – Gothic 3 Mod-Editor",
		version.CompanyName:      "AdanosEdit",
		version.LegalCopyright:   "© 2026 AdanosEdit – inoffizielles Fan-Werkzeug für Gothic 3",
		version.OriginalFilename: "AdanosEdit.exe",
		version.InternalName:     "AdanosEdit",
		version.ProductVersion:   os.Args[2],
		version.FileVersion:      os.Args[2],
	} {
		vi.Set(0x0407, k, v)
	}
	rs.SetVersionInfo(vi)
	out, err := os.Create(os.Args[3])
	if err != nil {
		panic(err)
	}
	defer out.Close()
	if err := rs.WriteObject(out, winres.ArchAMD64); err != nil {
		panic(err)
	}
}
