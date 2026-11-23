package app

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Spelunkern/Shaiya-GX/internal/data/sdata"
	"github.com/Spelunkern/Shaiya-GX/internal/data/text"
	"github.com/Spelunkern/Shaiya-GX/internal/proto"
)

func TestCraftValuesDecodeAndRejectInvalid(t *testing.T) {
	v, ok := parseCraftValues("01020304050607080953  ")
	if !ok || v.stats != [9]uint16{1, 2, 3, 4, 5, 6, 700, 800, 900} || v.enchant != 3 {
		t.Fatalf("unexpected values: %+v %t", v, ok)
	}
	for _, bad := range []string{"", "0102030405060708095", "010203040506070809x3"} {
		if _, ok := parseCraftValues(bad); ok {
			t.Fatalf("invalid craft accepted: %q", bad)
		}
	}
	if got := (&App{}).craftSummary("01000000000001000000"); got != "STR +1, HP +100" {
		t.Fatal(got)
	}
}

func TestItemDetailUsesInstanceAndGemDefinitions(t *testing.T) {
	def := &sdata.Item{Name: "Sword", Quality: 1200, Attack: 12}
	item := &proto.InventoryItem{Count: 1, Quality: 17, Craft: "01000000000000000002", Gems: [6]uint8{5, 9}}
	rows := itemDetailLines(def, item, 1, "Sell: 10 gold", func(id uint8) string {
		if id == 5 {
			return "Power Lapis"
		}
		return ""
	})
	var texts []string
	for _, row := range rows {
		texts = append(texts, row.text)
	}
	joined := strings.Join(texts, "|")
	if strings.Contains(joined, "endurance") || strings.Contains(joined, "Durability") {
		t.Fatal("legacy durability remains visible", joined)
	}
	for _, want := range []string{"Sword [2]", "STR +1", "Lapis: Power Lapis", "Lapis: Lapis 9"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %q", want, joined)
		}
	}
	shop := itemDetailLines(def, nil, 1, "Buy: 100 gold", nil)
	for _, row := range shop {
		if strings.Contains(row.text, "endurance") || strings.Contains(row.text, "Lapis:") {
			t.Fatal("shop definition invented mutable instance state")
		}
	}
}

func TestItemDescriptionRemovesColorControls(t *testing.T) {
	got := stripItemColors("{c5}Enhancement{/c}: {custom} +2")
	if got != "Enhancement: {custom} +2" {
		t.Fatal(got)
	}
}

func TestTooltipPreservesInlineColorsAndCompleteDescription(t *testing.T) {
	glyphs := tooltipGlyphs("Plain {c3}gold{/c} tail", white)
	var plain strings.Builder
	yellow := 0
	for _, g := range glyphs {
		plain.WriteRune(g.char)
		if g.color == ([4]float32{1, 1, 0, 1}) {
			yellow++
		}
	}
	if plain.String() != "Plain gold tail" || yellow != 4 {
		t.Fatal("inline description colours lost", plain.String(), yellow)
	}
	rows := itemDetailLines(&sdata.Item{Name: "Item", Description: "first\\n{c3}second{/c}\nthird"}, nil, 1, "", nil)
	if len(rows) != 4 || rows[2].text != "{c3}second{/c}" || rows[3].text != "third" {
		t.Fatal("item description was truncated", rows)
	}
}

func TestEquipmentTooltipHasNoModeRequirement(t *testing.T) {
	rows := itemDetailLines(&sdata.Item{Name: "Goddess Anor Helmet", Grow: 3}, nil, 1, "", nil)
	for _, row := range rows {
		if strings.Contains(row.text, "Required mode") {
			t.Fatal("obsolete mode requirement visible")
		}
	}
}

func TestLapisTooltipTargetsFollowPlacementFlags(t *testing.T) {
	for _, typ := range []uint8{30, 95} {
		def := &sdata.Item{Type: typ, Name: "Gem", MinLevel: 1, Country: 1,
			Classes: [6]uint8{1, 0, 1, 0, 1, 0}, Grow: 1,
			RequiredStats: [6]uint16{1}, Description: "original description"}
		rows := itemDetailLines(def, nil, 1, "", nil)
		var joined string
		for _, row := range rows {
			joined += "|" + row.text
		}
		for _, want := range []string{"Item possible to equip", "Weapons", "Helmets", "Upper armor", "Gauntlet", "Shields", "Necklaces", "Bracelets"} {
			if !strings.Contains(joined, "|"+want) {
				t.Fatalf("type %d missing %s: %s", typ, want, joined)
			}
		}
		for _, absent := range []string{"Lower armor", "Shoes", "Rings", "level 1"} {
			if strings.Contains(joined, "|"+absent) {
				t.Fatalf("type %d advertises unsupported %s", typ, absent)
			}
		}
		if strings.Contains(joined, "|Coat") {
			t.Fatalf("type %d has incorrect cape target: %s", typ, joined)
		}
		if rows[len(rows)-1].text != def.Description {
			t.Fatal("description must remain last for visual tooltip splitting")
		}
	}
}

func TestSpeedTooltipUsesNativeFieldsActiveLapisAndSysMsg(t *testing.T) {
	a := inputApp()
	a.msg = map[int]string{5197: "ATTACK NORM", 5276: "NORM", 5198: "ATTACK QUICK", 5209: "MOVE UP"}
	weapon := &sdata.Item{Type: 1, TypeID: 1, Name: "Test weapon", AttackTime: 5, Speed: 1, Attribute: 1}
	a.items = map[sdata.ItemKey]*sdata.Item{
		{Type: 1, TypeID: 1}:  weapon,
		{Type: 30, TypeID: 1}: {Type: 30, TypeID: 1, Name: "Active speed", AttackTime: 1, Speed: 2},
		{Type: 30, TypeID: 2}: {Type: 30, TypeID: 2, Name: "Inactive element", Attribute: 2, AttackTime: 3, Speed: 3},
	}
	it := proto.InventoryItem{Type: 1, TypeID: 1, Gems: [6]uint8{1, 2}}
	a.drawItemDetails(weapon, &it, 1, 0, 0, 1024, 768, "")
	has := func(want string) bool {
		for _, r := range a.tooltip {
			if r.text == want {
				return true
			}
		}
		return false
	}
	if !has("ATTACK QUICK") || !has("MOVE UP +3") {
		t.Fatal("native/socket speed missing or inactive lapis applied", a.tooltip)
	}
	if weapon.AttackTime != 5 || weapon.Speed != 1 {
		t.Fatal("tooltip mutated cached definition")
	}
	for _, d := range []*sdata.Item{{Type: 22, AttackTime: 1}, {Type: 30, AttackTime: 2}} {
		rows := itemDetailLines(d, nil, 1, "", nil, a.msg)
		found := false
		for _, r := range rows {
			found = found || r.text == "ATTACK +"+fmt.Sprint(d.AttackTime)
		}
		if !found {
			t.Fatal("native attack bonus missing", rows)
		}
	}
	rows := itemDetailLines(&sdata.Item{Type: 25, AttackTime: 1, Speed: 9}, nil, 1, "", nil, a.msg)
	for _, r := range rows {
		if strings.HasPrefix(r.text, "ATTACK") || strings.HasPrefix(r.text, "MOVE UP") {
			t.Fatal("consumable skill level displayed as equipment speed")
		}
	}
}

func TestEquipmentAbsorptionTooltip(t *testing.T) {
	for _, tt := range []struct {
		typ, absorb uint8
		want        bool
	}{
		{17, 30, true}, {1, 255, true}, {40, 10, true}, {17, 0, false},
		{25, 30, false}, {30, 30, true}, {100, 30, false},
	} {
		def := &sdata.Item{Type: tt.typ, ExpAbsorb: tt.absorb}
		rows := itemDetailLines(def, nil, 1, "", nil, text.SysMsg{485: "ABSORB"})
		found := false
		for _, row := range rows {
			found = found || row.text == fmt.Sprintf("ABSORB %d", tt.absorb)
		}
		if found != tt.want {
			t.Fatalf("type=%d Exp=%d: absorption shown=%v, want %v", tt.typ, tt.absorb, found, tt.want)
		}
	}
}

func TestAbsorptionTooltipTotalsNativeLapisAndEnchant(t *testing.T) {
	a := inputApp()
	a.msg = text.SysMsg{485: "ABSORB"}
	def := &sdata.Item{Type: 17, TypeID: 1, ExpAbsorb: 30, Attribute: 1, Server: 1, RequiredStats: [6]uint16{0, 0, 0, 0, 1}, Description: "native description"}
	a.items = map[sdata.ItemKey]*sdata.Item{
		{Type: 30, TypeID: 1}: {Type: 30, ExpAbsorb: 255},
		{Type: 30, TypeID: 2}: {Type: 30, ExpAbsorb: 99, Attribute: 2},
	}
	it := proto.InventoryItem{Type: 17, TypeID: 1, Craft: "00000000000000000053", Gems: [6]uint8{1, 1, 2, 99}}
	a.drawItemDetails(def, &it, 1, 0, 0, 1024, 768, "")
	rows := map[string]bool{}
	for _, row := range a.tooltip {
		rows[row.text] = true
	}
	for _, want := range []string{"ABSORB 555"} {
		if !rows[want] {
			t.Fatalf("missing %q: %v", want, a.tooltip)
		}
	}
	if def.ExpAbsorb != 30 || a.tooltipDescription != "native description" {
		t.Fatal("cached definition or description changed")
	}
	// Neither weapon enchant encoding nor ineligible crafts grant armor absorption.
	for _, craft := range []string{"00000000000000000003", "00000000000000000071", "invalid"} {
		it.Craft = craft
		for _, row := range itemDetailLines(def, &it, 1, "", nil) {
			if row.text == "Absorption 45" {
				t.Fatalf("invalid armor craft %q displayed absorption", craft)
			}
		}
	}
	it.Craft = "00000000000000000053"
	def.RequiredStats[4] = 100
	for _, row := range itemDetailLines(def, &it, 1, "", nil) {
		if row.text == "Absorption 45" {
			t.Fatal("ineligible craft displayed absorption")
		}
	}
}

func TestAbsorptionTooltipArmor20AndLapis60(t *testing.T) {
	a := inputApp()
	a.msg = text.SysMsg{485: "Absorption"}
	a.items = map[sdata.ItemKey]*sdata.Item{{Type: 30, TypeID: 1}: {Type: 30, ExpAbsorb: 60}}
	def := &sdata.Item{Type: 17, Name: "Armor", Server: 1, RequiredStats: [6]uint16{0, 0, 0, 0, 1}}
	it := proto.InventoryItem{Type: 17, Craft: "00000000000000000070", Gems: [6]uint8{1}}
	for _, tt := range []struct {
		native uint8
		craft  string
		gems   [6]uint8
		want   string
	}{
		{0, it.Craft, it.Gems, "Absorption 160"},
		{30, it.Craft, it.Gems, "Absorption 190"},
		{0, "", it.Gems, "Absorption 60"},
		{0, it.Craft, [6]uint8{}, "Absorption 100"},
	} {
		def.ExpAbsorb, it.Craft, it.Gems = tt.native, tt.craft, tt.gems
		a.drawItemDetails(def, &it, 1, 0, 0, 1024, 768, "")
		count := 0
		for _, row := range a.tooltip {
			if strings.HasPrefix(row.text, "Absorption") {
				count++
				if row.text != tt.want {
					t.Fatalf("got %q want %q", row.text, tt.want)
				}
			}
			if strings.Contains(row.text, "(Lapis)") || strings.Contains(row.text, "(Enchant)") {
				t.Fatal("split absorption row", row)
			}
		}
		if count != 1 {
			t.Fatalf("absorption rows=%d", count)
		}
		if tt.craft != "" && a.tooltip[0].text != "Armor [20]" {
			t.Fatal("enchant level missing from name", a.tooltip[0])
		}
	}
}

func TestAbsorptionTooltipUsesServerIniBonuses(t *testing.T) {
	a := inputApp()
	a.msg = text.SysMsg{485: "Absorption"}
	def := &sdata.Item{Type: 17, Name: "Armor", Server: 1, RequiredStats: [6]uint16{0, 0, 0, 0, 1}, ExpAbsorb: 30}
	a.items = map[sdata.ItemKey]*sdata.Item{{Type: 30, TypeID: 1}: {Type: 30, ExpAbsorb: 60}}
	rules := proto.EnchantRules{MaxLevel: 49}
	rules.ArmorBonus[20] = 777
	rules.ArmorBonus[49] = 888
	if _, err := a.state.Apply(proto.EnchantRulesPacket(rules)); err != nil {
		t.Fatal(err)
	}
	it := proto.InventoryItem{Type: 17, Craft: "00000000000000000070", Gems: [6]uint8{1}}
	for _, tt := range []struct{ craft, want string }{{"00000000000000000070", "Absorption 867"}, {"00000000000000000099", "Absorption 978"}} {
		it.Craft = tt.craft
		a.drawItemDetails(def, &it, 1, 0, 0, 1024, 768, "")
		found := 0
		for _, row := range a.tooltip {
			if strings.HasPrefix(row.text, "Absorption ") {
				found++
				if row.text != tt.want {
					t.Fatal(row.text, tt.want)
				}
			}
		}
		if found != 1 {
			t.Fatal("total absorption row missing/duplicated", a.tooltip)
		}
	}
	// A configured zero replaces the old hardcoded +100, including removing
	// the row entirely when there is no native or lapis contribution.
	def.ExpAbsorb = 0
	it.Gems = [6]uint8{}
	it.Craft = "00000000000000000070"
	a.state.EnchantRules.ArmorBonus[20] = 0
	a.drawItemDetails(def, &it, 1, 0, 0, 1024, 768, "")
	for _, row := range a.tooltip {
		if strings.HasPrefix(row.text, "Absorption ") {
			t.Fatal("zero configured bonus displayed", row)
		}
	}
}

func TestItemClassWarningUsesSysMsgOnlyForRestrictedEquipment(t *testing.T) {
	a := inputApp()
	a.state.SelfID = 1
	a.state.Slots[0] = proto.CharacterSlot{ID: 1, Class: 4}
	a.msg = text.SysMsg{456: "Clase no disponible"}
	def := &sdata.Item{Type: 17, Classes: [6]uint8{1}}
	line, ok := a.itemClassWarning(def)
	if !ok || line.text != "Clase no disponible" || line.color != [4]float32{1, 0, 0, 1} {
		t.Fatalf("warning = %+v, present = %t", line, ok)
	}
	def.Classes[4] = 1
	if _, ok := a.itemClassWarning(def); ok {
		t.Fatal("compatible equipment displayed a warning")
	}
	if _, ok := a.itemClassWarning(&sdata.Item{Type: 25}); ok {
		t.Fatal("consumable displayed an equipment warning")
	}
}

func TestLapisTooltipTotalsWeaponAndStatBonuses(t *testing.T) {
	a := inputApp()
	a.msg = text.SysMsg{5191: "Attack power", 5208: "max."}
	weapon := &sdata.Item{
		Type: 1, TypeID: 1, Name: "Sword",
		Attack: 50, AttackAdd: 15,
		RecoveryHP: 100, RecoverySP: 50, RecoveryMP: 25,
		Stats: [6]uint16{5, 0, 3, 0, 0, 2},
	}
	a.items = map[sdata.ItemKey]*sdata.Item{
		{Type: 1, TypeID: 1}:  weapon,
		{Type: 30, TypeID: 1}: {Type: 30, TypeID: 1, Attack: 8, Stats: [6]uint16{10, 4}},
		{Type: 30, TypeID: 2}: {Type: 30, TypeID: 2, AttackAdd: 12, RecoveryHP: 200, RecoverySP: 75, RecoveryMP: 40, Stats: [6]uint16{0, 0, 7, 6, 5, 8}},
	}
	it := proto.InventoryItem{
		Type: 1, TypeID: 1,
		Craft: "04000000000003000000",
		Gems:  [6]uint8{1, 2},
	}
	a.drawItemDetails(weapon, &it, 1, 0, 0, 1024, 768, "")

	yellow := [4]float32{1, 1, 0, 1}
	for _, want := range []itemDetailLine{
		{"Attack power 58 - 85", yellow},
		{"max. HP +300  {r}(+300){/r}", yellow},
		{"max. SP +125", yellow},
		{"max. MP +65", yellow},
		{"STR +15  {r}(+4){/r}", yellow},
		{"DEX +4", yellow},
		{"REC +10", yellow},
		{"INT +6", yellow},
		{"WIS +5", yellow},
		{"LUC +10", yellow},
	} {
		if !slices.Contains(a.tooltip, want) {
			t.Fatalf("missing %+v in tooltip: %+v", want, a.tooltip)
		}
	}
	if weapon.Attack != 50 || weapon.AttackAdd != 15 || weapon.Stats[0] != 5 || weapon.RecoveryHP != 100 {
		t.Fatal("tooltip mutated cached item definition")
	}
}

func TestLapisTooltipDefenseResistAndInactiveElement(t *testing.T) {
	a := inputApp()
	a.msg = text.SysMsg{5202: "Defense", 5203: "Resistance"}
	armor := &sdata.Item{Type: 17, TypeID: 1, Name: "Armor", Attribute: 1, Def: 45, Resist: 10, Stats: [6]uint16{0, 0, 5}}
	a.items = map[sdata.ItemKey]*sdata.Item{
		{Type: 17, TypeID: 1}: armor,
		{Type: 30, TypeID: 1}: {Type: 30, TypeID: 1, Def: 12, Stats: [6]uint16{0, 0, 9}},
		{Type: 30, TypeID: 2}: {Type: 30, TypeID: 2, Resist: 20},
		{Type: 30, TypeID: 3}: {Type: 30, TypeID: 3, Attribute: 2, Def: 99, Resist: 99, Stats: [6]uint16{99}},
	}
	it := proto.InventoryItem{Type: 17, TypeID: 1, Gems: [6]uint8{1, 2, 3}}
	a.drawItemDetails(armor, &it, 1, 0, 0, 1024, 768, "")

	for _, want := range []itemDetailLine{
		{"Defense 45 +12", white},
		{"Resistance 10 +20", white},
		{"REC +14", [4]float32{1, 1, 0, 1}},
	} {
		if !slices.Contains(a.tooltip, want) {
			t.Fatalf("missing %+v in tooltip: %+v", want, a.tooltip)
		}
	}
	for _, row := range a.tooltip {
		if strings.Contains(row.text, "STR +99") {
			t.Fatalf("inactive elemental lapis leaked stats: %+v", row)
		}
	}

	// Base Def/Resist 0 still surfaces active lapis defense/resistance.
	zeroBase := &sdata.Item{Type: 16, TypeID: 2, Name: "Helmet"}
	a.items[sdata.ItemKey{Type: 16, TypeID: 2}] = zeroBase
	it = proto.InventoryItem{Type: 16, TypeID: 2, Gems: [6]uint8{1, 2}}
	a.drawItemDetails(zeroBase, &it, 1, 0, 0, 1024, 768, "")
	for _, want := range []itemDetailLine{{"Defense +12", white}, {"Resistance +20", white}} {
		if !slices.Contains(a.tooltip, want) {
			t.Fatalf("missing zero-base lapis row %+v in %+v", want, a.tooltip)
		}
	}

	// Cloak emblem bytes in Gems must never be treated as lapis stats.
	cloak := &sdata.Item{Type: 24, TypeID: 1, Name: "Cloak", Def: 5}
	it = proto.InventoryItem{Type: 24, TypeID: 1, Gems: [6]uint8{1, 2}}
	a.drawItemDetails(cloak, &it, 1, 0, 0, 1024, 768, "")
	if !slices.Contains(a.tooltip, itemDetailLine{"Defense 5", white}) {
		t.Fatalf("cloak emblem bytes modified tooltip: %+v", a.tooltip)
	}
}

func TestWeaponEnchantTooltipUsesDefaultAndServerRulesWithLapis(t *testing.T) {
	a := inputApp()
	a.msg = text.SysMsg{5191: "Attack power"}
	weapon := &sdata.Item{
		Type: 1, TypeID: 1, Name: "Sword",
		Attack: 50, AttackAdd: 15,
		Server: 6, RequiredStats: [6]uint16{0, 0, 0, 0, 3},
	}
	a.items = map[sdata.ItemKey]*sdata.Item{
		{Type: 1, TypeID: 1}:  weapon,
		{Type: 30, TypeID: 1}: {Type: 30, TypeID: 1, Attack: 8},
		{Type: 30, TypeID: 2}: {Type: 30, TypeID: 2, AttackAdd: 12},
	}
	it := proto.InventoryItem{
		Type: 1, TypeID: 1,
		Craft: "00000000000000000003", // +3 default bonus = 21
		Gems:  [6]uint8{1, 2},
	}
	yellow := [4]float32{1, 1, 0, 1}
	a.drawItemDetails(weapon, &it, 1, 0, 0, 1024, 768, "")
	if !slices.Contains(a.tooltip, itemDetailLine{"Attack power 79 - 106", yellow}) {
		t.Fatalf("expected weapon +3 + lapis attack 79 - 106, got %+v", a.tooltip)
	}

	rules := proto.EnchantRules{MaxLevel: 49}
	rules.WeaponBonus[3] = 50
	rules.WeaponBonus[49] = 900
	if _, err := a.state.Apply(proto.EnchantRulesPacket(rules)); err != nil {
		t.Fatal(err)
	}
	a.drawItemDetails(weapon, &it, 1, 0, 0, 1024, 768, "")
	if !slices.Contains(a.tooltip, itemDetailLine{"Attack power 108 - 135", yellow}) {
		t.Fatalf("expected server WeaponBonus[3]=50 attack 108 - 135, got %+v", a.tooltip)
	}

	it.Craft = "00000000000000000049"
	it.Gems = [6]uint8{}
	a.drawItemDetails(weapon, &it, 1, 0, 0, 1024, 768, "")
	if !slices.Contains(a.tooltip, itemDetailLine{"Attack power 950 - 965", yellow}) {
		t.Fatalf("expected server WeaponBonus[49]=900 attack 950 - 965, got %+v", a.tooltip)
	}

	// Configured zero bonus replaces hardcoded fallback, leaving non-lapis attack white.
	a.state.EnchantRules.WeaponBonus[3] = 0
	it.Craft = "00000000000000000003"
	a.drawItemDetails(weapon, &it, 1, 0, 0, 1024, 768, "")
	if !slices.Contains(a.tooltip, itemDetailLine{"Attack power 50 - 65", white}) {
		t.Fatalf("expected zero configured WeaponBonus attack 50 - 65 white, got %+v", a.tooltip)
	}

	// Armor enchant encoding (>= 50) never adds weapon attack bonus.
	it.Craft = "00000000000000000053"
	a.drawItemDetails(weapon, &it, 1, 0, 0, 1024, 768, "")
	if !slices.Contains(a.tooltip, itemDetailLine{"Attack power 50 - 65", white}) {
		t.Fatalf("armor enchant encoding modified weapon attack: %+v", a.tooltip)
	}
}
