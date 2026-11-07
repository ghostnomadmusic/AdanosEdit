# AdanosEdit – Gothic 3 Mod-Editor

**AdanosEdit** is a Windows editor for *Gothic 3* mods: change the game world in
3D, create NPCs, dialogs and quests, and save everything as a mod for the
Gothic 3 Mod Starter. The original game files are never modified.

*Deutsch weiter unten.*

**Download:** see [Releases](../../releases) – unpack the zip and start
`AdanosEdit.exe`. Full instructions are in the program (F1) and in
`Readme.txt` of the release.

## Features

- 3D view of the whole game world (Myrtana, Nordmar, Varant)
- Move, rotate, copy, delete objects and put them on the ground – one at a
  time or several at once (Ctrl+click, frame selection); undo and redo
- Place new objects and items from all game templates
- Create NPCs: appearance, group and enclave, inventory, trader wares,
  daily routines; set up traders and teachers
- Write dialogs and change original ones: conditions across NPCs,
  branching conversations, item and gold conditions, actions (gold, items,
  experience, events, routines, trading, teaching), disable dialogs
- Quests (bring, report, kill, defeat, drive away, arena, reach a place,
  rob a chest, escort, follow, cast a spell) with generated dialogs, quest
  chains, rewards and time limits
- Custom zones, written into the mod's navigation map
- Chests and locks, "Go to" for towns and camps, bulk removal of NPCs
- "Check mod" before saving
- User interface in German, English, Russian and Polish

## Requirements

- Windows 64 bit, OpenGL 1.5 or newer
- *Gothic 3* with **Community Patch 1.75** (tested with the Steam version and
  CP 1.75.14)
- **Gothic 3 Mod Starter** (1.2.0.1 for the Community Patch recommended)

## Building from source

AdanosEdit is written in Go and uses only the standard library and the
Windows API (no cgo).

```
# Go 1.24 or newer
go build -ldflags "-H windowsgui -s -w" -o AdanosEdit.exe .
```

Cross-compiling from Linux/macOS:
`GOOS=windows GOARCH=amd64 go build -ldflags "-H windowsgui -s -w" -o AdanosEdit.exe .`

Project layout:

| Path | Content |
|------|---------|
| `*.go` | user interface (Win32), rendering (OpenGL), editor logic |
| `g3/` | Gothic 3 file formats: archives (.pak), world files, templates, meshes, dialogs, quests, navigation map |
| `scene/` | world model, spatial queries, loader |
| `lang/` | translations (`table.go`: German text → English, Russian, Polish) |
| `cmd/i18ncheck` | lists texts missing a translation: `go run ./cmd/i18ncheck x` |
| `tools/mkres` | regenerates `rsrc_windows_amd64.syso` (icon, version info) |

## License

MIT – see [LICENSE](LICENSE). Credits: see [NOTICE.md](NOTICE.md).
*Gothic 3* is a trademark of its respective owners; AdanosEdit is an
unofficial fan tool and contains no game data.

---

## Deutsch

**AdanosEdit** ist ein Windows-Editor für *Gothic-3*-Mods: Spielwelt in 3D
verändern, NPCs, Dialoge und Quests erstellen und alles als Mod für den
Gothic 3 Mod Starter speichern. Die Originaldateien des Spiels bleiben
unverändert.

**Download:** unter [Releases](../../releases) – Zip entpacken und
`AdanosEdit.exe` starten. Die ausführliche Anleitung steht im Programm (F1)
und in der `Readme.txt` des Releases.

**Voraussetzungen:** Windows 64 Bit, *Gothic 3* mit **Community Patch 1.75**
(getestet: Steam-Version mit CP 1.75.14) und der **Gothic 3 Mod Starter**
(empfohlen: 1.2.0.1 für den Community Patch).

**Selbst bauen:** Go 1.24 oder neuer, dann
`go build -ldflags "-H windowsgui -s -w" -o AdanosEdit.exe .`

Fehler und Wünsche gerne als [Issue](../../issues) melden – bei Problemen
bitte die `AdanosEdit.log` (neben der exe) anhängen.
