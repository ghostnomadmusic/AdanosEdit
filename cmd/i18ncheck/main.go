// i18ncheck listet alle zu uebersetzenden Texte und prueft die Tabelle.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"g3viewer/lang"
)

func main() {
	keys := map[string]bool{}
	files, _ := filepath.Glob("*.go")
	fset := token.NewFileSet()
	consts := map[string]string{}
	var parsed []*ast.File
	for _, f := range files {
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			panic(err)
		}
		parsed = append(parsed, af)
		ast.Inspect(af, func(n ast.Node) bool {
			if vs, ok := n.(*ast.ValueSpec); ok {
				for i, nm := range vs.Names {
					if i < len(vs.Values) {
						if bl, ok := vs.Values[i].(*ast.BasicLit); ok && bl.Kind == token.STRING {
							v, _ := strconv.Unquote(bl.Value)
							consts[nm.Name] = v
						}
					}
				}
			}
			return true
		})
	}
	for _, af := range parsed {
		ast.Inspect(af, func(n ast.Node) bool {
			ce, ok := n.(*ast.CallExpr)
			if !ok || len(ce.Args) != 1 {
				return true
			}
			id, ok := ce.Fun.(*ast.Ident)
			if !ok || (id.Name != "T" && id.Name != "tr") {
				return true
			}
			switch x := ce.Args[0].(type) {
			case *ast.BasicLit:
				v, _ := strconv.Unquote(x.Value)
				keys[v] = true
			case *ast.Ident:
				if v, ok := consts[x.Name]; ok {
					keys[v] = true
				}
			}
			return true
		})
	}
	for _, k := range lang.Extra {
		keys[k] = true
	}
	var ks []string
	for k := range keys {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	missing := 0
	for _, k := range ks {
		if !lang.Has(k) {
			missing++
			if len(os.Args) > 1 {
				fmt.Printf("%q\n", k)
			}
		}
	}
	fmt.Println("Texte:", len(ks), "fehlend:", missing, "Tabelle:", lang.Count())
	for _, k := range lang.Keys() {
		if !keys[k] {
			fmt.Printf("unbenutzt: %q\n", k)
		}
	}
	for _, p := range lang.FormatProblems() {
		fmt.Println("Format:", p)
	}
}
