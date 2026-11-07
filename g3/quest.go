package g3

import (
	"strconv"
	"strings"
)

// Quest-Arten (Type)
const (
	QuestBring     = 0  // Gegenstaende bringen
	QuestReport    = 1  // jemandem etwas ausrichten
	QuestKill      = 2  // jemanden toeten
	QuestDefeat    = 3  // jemanden besiegen (niederschlagen, nicht toeten)
	QuestDriveAway = 4  // jemanden vertreiben (besiegen, danach verschwindet er)
	QuestArena     = 5  // Arenakampf (DestinationEntity = Arena-Zone)
	QuestReach     = 8  // Ort erreichen (DeliveryEntities betritt die Zone DestinationEntity)
	QuestRansack   = 11 // Truhe ausrauben (Gegenstand + Anzahl aus der Truhe DestinationEntity)
	QuestFree      = 10 // Ort befreien (DeliveryEntities = Enklaven, deren Besatzer besiegt werden)
	QuestSpell     = 13 // Zauber auf jemanden wirken (SpellCast: Zauber SpellName auf DeliveryEntities)
	// Begleit-Quests
	QuestBringNPC = 6 // NPC folgt dem Helden zu einer Person (DeliveryEntities -> DestinationEntity)
	QuestFollow   = 7 // Held folgt einem NPC zu einem Ort (DeliveryEntities fuehrt zu DestinationEntity)
)

// Bedingungsarten fuer Gespraeche, die zu einer Quest gehoeren
const (
	CondOpen      = 5  // Quest angeboten, noch nicht angenommen
	CondActivator = 6  // Gespraech startet die Quest
	CondDelivery  = 8  // Bedingung erfuellt (Gegenstaende da, Person erreicht)
	CondOvertime  = 4  // Zeitlimit abgelaufen
	CondFailed    = 12 // Quest gescheitert
	CondRerun     = 21 // laeuft, wiederholbar (z. B. Arenakampf erneut)
)

// Quest ist eine Gothic-3-Quest (.quest). Alle Felder bleiben in ihrer
// Reihenfolge erhalten; bekannte werden ueber Get/Set gelesen und geschrieben.
type Quest struct {
	keys []string
	vals map[string]string
	eq   map[string]bool // Zeile hatte ein "="
}

// QuestPath ist der Pfad der Quest-Datei im Archiv Quests.
func QuestPath(name string) string {
	return "G3_World_01/" + name + "_quest_G3_World_01.quest"
}

// NewQuest legt eine Quest mit allen Feldern (Standardwerten) an.
func NewQuest(name string) *Quest {
	q := &Quest{vals: map[string]string{}, eq: map[string]bool{}}
	for _, kv := range [][2]string{
		{"Name", name}, {"Type", "0"}, {"FinishedQuests", ""}, {"LogTopic", ""}, {"LogText", ""},
		{"ExperiencePoints", "0"}, {"PoliticalSuccess", "0"}, {"PoliticalSuccessAmount", "0"},
		{"EnclaveSuccess", ""}, {"EnclaveSuccessAmount", "0"}, {"JobSuccess", "0"}, {"JobSuccessAmount", "0"},
		{"Folder", "G3Viewer/"}, {"DeliveryEntities", ""}, {"DeliveryAmounts", ""}, {"DeliveryCounter", "0"},
		{"DestinationEntity", ""}, {"RunningTimeYears", "0"}, {"RunningTimeDays", "0"}, {"RunningTimeHours", "0"},
		{"AttribSuccess", ""}, {"AttribSuccessAmount", "0"},
	} {
		q.Set(kv[0], kv[1])
	}
	return q
}

// ParseQuest liest eine Quest-Datei.
func ParseQuest(d []byte) *Quest {
	q := &Quest{vals: map[string]string{}, eq: map[string]bool{}}
	for _, l := range strings.Split(string(d), "\n") {
		l = strings.TrimRight(l, "\r")
		if l == "" || strings.HasPrefix(l, "[") {
			continue
		}
		k, v, has := strings.Cut(l, "=")
		if _, ok := q.vals[k]; !ok {
			q.keys = append(q.keys, k)
		}
		q.vals[k] = v
		q.eq[k] = has
	}
	return q
}

func (q *Quest) Get(k string) string { return q.vals[k] }

func (q *Quest) GetInt(k string) int {
	v, _ := strconv.Atoi(q.vals[k])
	return v
}

func (q *Quest) Set(k, v string) {
	if _, ok := q.vals[k]; !ok {
		q.keys = append(q.keys, k)
	}
	if q.vals[k] != v {
		q.eq[k] = v != ""
	}
	q.vals[k] = v
}

func (q *Quest) SetInt(k string, v int) { q.Set(k, strconv.Itoa(v)) }

func (q *Quest) Name() string { return q.vals["Name"] }

// Clone liefert eine unabhaengige Kopie.
func (q *Quest) Clone() *Quest {
	c := &Quest{keys: append([]string(nil), q.keys...), vals: map[string]string{}, eq: map[string]bool{}}
	for k, v := range q.vals {
		c.vals[k] = v
	}
	for k, v := range q.eq {
		c.eq[k] = v
	}
	return c
}

// Bytes erzeugt den Dateiinhalt (CRLF wie die Originale).
func (q *Quest) Bytes() []byte {
	var b strings.Builder
	b.WriteString("[Quest]\r\n")
	for _, k := range q.keys {
		if v := q.vals[k]; v != "" || q.eq[k] {
			b.WriteString(k + "=" + v + "\r\n")
		} else {
			b.WriteString(k + "\r\n")
		}
	}
	return []byte(b.String())
}
