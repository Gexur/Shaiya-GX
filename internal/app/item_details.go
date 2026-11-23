package app

import (
	"fmt"
	"strings"

	"github.com/Spelunkern/Shaiya-GX/internal/attackspeed"
	"github.com/Spelunkern/Shaiya-GX/internal/data/sdata"
	"github.com/Spelunkern/Shaiya-GX/internal/data/text"
	"github.com/Spelunkern/Shaiya-GX/internal/itemtype"
	"github.com/Spelunkern/Shaiya-GX/internal/proto"
)

type craftValues struct {
	stats   [9]uint16
	enchant uint8
}

// Craftname is ten decimal pairs: six attributes, HP/MP/SP in hundreds,
// then enchantment. Armor enchantment is encoded as 50 plus the level.
func parseCraftValues(name string) (craftValues, bool) {
	var out craftValues
	name = strings.TrimRight(name, " ")
	if len(name) != 20 {
		return out, false
	}
	var values [10]uint16
	for n := range values {
		a, b := name[n*2], name[n*2+1]
		if a < '0' || a > '9' || b < '0' || b > '9' {
			return craftValues{}, false
		}
		values[n] = uint16(a-'0')*10 + uint16(b-'0')
	}
	copy(out.stats[:], values[:9])
	for n := 6; n < 9; n++ {
		out.stats[n] *= 100
	}
	switch level := values[9]; {
	case level <= 49:
		out.enchant = uint8(level)
	case level > 50 && level <= 99:
		out.enchant = uint8(level - 50)
	}
	return out, true
}

func (a *App) craftSummary(name string) string {
	if strings.TrimSpace(name) == "" {
		return a.tr(12251)
	}
	v, ok := parseCraftValues(name)
	if !ok {
		return a.tr(12252)
	}
	names := [9]string{"STR", "DEX", "REC", "INT", "WIS", "LUC", "HP", "MP", "SP"}
	var parts []string
	for i, n := range v.stats {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%s +%d", names[i], n))
		}
	}
	if len(parts) == 0 {
		return a.tr(12251)
	}
	return strings.Join(parts, ", ")
}

type itemDetailLine struct {
	text  string
	color [4]float32
}

// Plain-text consumers of Item.SData omit its inline colour controls.
func stripItemColors(text string) string {
	var out strings.Builder
	for i := 0; i < len(text); {
		if strings.HasPrefix(text[i:], "{/c}") {
			i += 4
			continue
		}
		if strings.HasPrefix(text[i:], "{c") {
			end := i + 2
			for end < len(text) && text[end] >= '0' && text[end] <= '9' {
				end++
			}
			if end > i+2 && end < len(text) && text[end] == '}' {
				i = end + 1
				continue
			}
		}
		out.WriteByte(text[i])
		i++
	}
	return out.String()
}

func (a *App) itemClassWarning(def *sdata.Item) (itemDetailLine, bool) {
	if a.state == nil || len(inventoryEquipSlots(def.Type)) == 0 {
		return itemDetailLine{}, false
	}
	own, ok := a.selfSlot()
	if !ok || int(own.Class) >= len(def.Classes) || def.Classes[own.Class] != 0 {
		return itemDetailLine{}, false
	}
	label := a.tr(456)
	return itemDetailLine{label, [4]float32{1, 0, 0, 1}}, true
}

type lapisTooltipBonuses struct {
	attackChanged bool
	def, resist   uint32
	enchantRules  *proto.EnchantRules
}

func formatDefenseBonusLine(label string, base uint16, bonus uint32) string {
	switch {
	case base > 0 && bonus > 0:
		return fmt.Sprintf("%s %d +%d", label, base, bonus)
	case base > 0:
		return fmt.Sprintf("%s %d", label, base)
	case bonus > 0:
		return fmt.Sprintf("%s +%d", label, bonus)
	default:
		return ""
	}
}

// Native 0x4a3950 distinguishes equipment bonuses from consumable restoration.
// SysMsg owns labels; inline description controls remain archive data.
func itemDetailLines(def *sdata.Item, instance *proto.InventoryItem, count uint16, price string, gemName func(uint8) string, messages ...text.SysMsg) []itemDetailLine {
	return itemDetailLinesWithBonuses(def, instance, count, price, gemName, lapisTooltipBonuses{}, messages...)
}

func itemDetailLinesWithBonuses(def *sdata.Item, instance *proto.InventoryItem, count uint16, price string, gemName func(uint8) string, bonuses lapisTooltipBonuses, messages ...text.SysMsg) []itemDetailLine {
	instance = withoutCloakGems(instance)
	label := func(id int) string {
		if len(messages) > 0 {
			return messages[0].Lookup(id)
		}
		return text.Defaults[id]
	}
	lines := make([]itemDetailLine, 0, 20)
	add := func(text string, color [4]float32) {
		if text != "" {
			lines = append(lines, itemDetailLine{text, color})
		}
	}
	craft := craftValues{}
	if instance != nil {
		craft, _ = parseCraftValues(instance.Craft)
	}
	name := def.Name
	if craft.enchant > 0 {
		name = fmt.Sprintf("%s [%d]", name, craft.enchant)
	}
	add(name, [4]float32{1, 1, 0, 1})
	rt := itemtype.Real(def.Type)
	equipment := rt >= 1 && rt <= 25 && !itemHasRestoration(def.Type)
	if rt >= 1 && rt <= 15 {
		add(label(5192+int(attackspeed.Tier(def.AttackTime, 0))), white)
	} else if (equipment || def.Type == 30) && def.AttackTime > 0 {
		// SysMsg supplies the localized attack-speed prefix and the tier name.
		prefix := strings.TrimSpace(strings.Replace(label(5197), label(5276), "", 1))
		add(fmt.Sprintf("%s +%d", prefix, def.AttackTime), [4]float32{1, 1, 0, 1})
	}
	if (equipment || def.Type == 30) && def.Speed > 0 {
		add(fmt.Sprintf("%s +%d", label(5209), def.Speed), [4]float32{1, 1, 0, 1})
	}

	enchantAttack := itemWeaponEnchantAttack(def, instance, bonuses.enchantRules)
	if def.Attack != 0 || def.AttackAdd != 0 || enchantAttack != 0 {
		minAttack := uint32(def.Attack) + enchantAttack
		maxAttack := minAttack + uint32(def.AttackAdd)
		color := white
		if bonuses.attackChanged || enchantAttack != 0 {
			color = [4]float32{1, 1, 0, 1}
		}
		add(fmt.Sprintf("%s %d - %d", label(5191), minAttack, maxAttack), color)
	}
	add(formatDefenseBonusLine(label(5202), def.Def, bonuses.def), white)
	add(formatDefenseBonusLine(label(5203), def.Resist, bonuses.resist), white)
	if equipment || def.Type == 30 {
		if absorption := itemAbsorption(def, instance); absorption > 0 {
			add(fmt.Sprintf("%s %d", label(485), absorption), white)
		}
	}
	if def.Attribute >= 1 && def.Attribute <= 8 && proto.FirecrackerVariant(def.Type, def.TypeID) == 0 {
		add(elementName(def.Attribute, messages...), [4]float32{1, 1, 0, 1})
	}
	hp, mp, sp := def.RecoveryHP, def.RecoveryMP, def.RecoverySP
	if itemHasRestoration(def.Type) {
		hp, mp, sp = 0, 0, 0
	}
	specs := []struct {
		name   string
		base   uint16
		remake uint16
	}{{label(5208) + " HP", hp, craft.stats[6]}, {label(5208) + " SP", sp, craft.stats[8]}, {label(5208) + " MP", mp, craft.stats[7]}}
	for i, name := range [6]string{"STR", "DEX", "REC", "INT", "WIS", "LUC"} {
		specs = append(specs, struct {
			name   string
			base   uint16
			remake uint16
		}{name, def.Stats[i], craft.stats[i]})
	}
	yellow := [4]float32{1, 1, 0, 1}
	orange := [4]float32{1, 135.0 / 255, 0, 1}
	for _, stat := range specs {
		if stat.base > 0 {
			text := fmt.Sprintf("%s +%d", stat.name, stat.base)
			if stat.remake > 0 {
				text += fmt.Sprintf("  {r}(+%d){/r}", stat.remake)
			}
			add(text, yellow)
		}
	}
	for _, stat := range specs {
		if stat.base == 0 && stat.remake > 0 {
			add(fmt.Sprintf("%s +%d", stat.name, stat.remake), orange)
		}
	}
	if def.Type == 30 || def.Type == 95 {
		add(label(5181), yellow)
		// Use the same native target flags as placement. These fields are
		// overloaded on lapis/lapisias, rather than character requirements.
		for _, target := range []struct {
			typ uint8
			msg int
		}{
			{1, 5182}, {16, 5183},
			{17, 5184}, {18, 5185},
			{21, 5186}, {19, 5187},
			{20, 5188}, {22, 5189},
			{23, 5190},
			{40, 5381},
		} {
			if lapFitsItem(def, target.typ) {
				add(label(target.msg), white)
			}
		}
	} else if def.MinLevel > 0 {
		add(fmt.Sprintf("%s %d", label(5211), def.MinLevel), white)
	}

	if rest, ok := strings.CutPrefix(price, "HouseBuy:"); ok {
		// 0x4a3950: inside a guild house a merchant tooltip shows the reduced
		// price under SysMsg 8025 (red when unaffordable), then the normal price in grey.
		parts := strings.Split(rest, "|")
		if len(parts) == 3 {
			color := white
			if parts[0] == "short" {
				color = [4]float32{1, 0, 0, 1}
			}
			add(label(8025)+" : "+parts[1], color)
			add(label(5220)+" : "+parts[2], [4]float32{150.0 / 255, 150.0 / 255, 150.0 / 255, 1})
		}
	} else if price != "" && price != "Sell: 0 gold" {
		color := white
		if strings.HasPrefix(price, "Buy:") {
			price = strings.TrimPrefix(price, "Buy:")
			price = label(5220) + " :" + price
		} else if strings.HasPrefix(price, "Sell:") {
			price = strings.TrimPrefix(price, "Sell:")
			price = label(5219) + " :" + price
		}
		price = strings.TrimSuffix(price, " gold")
		add(price, color)
	}
	// Keep names available to nonvisual consumers; the tooltip places sockets
	// after its description, using the confirmed gem instance IDs.
	if instance != nil {
		for _, id := range instance.Gems {
			if id == 0 {
				continue
			}
			name := ""
			if gemName != nil {
				name = gemName(id)
			}
			if name == "" {
				name = strings.ReplaceAll(label(12303), "<v>", fmt.Sprint(id))
			}
			add("Lapis: "+name, white)
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(def.Description, `\n`, "\n"), "\n") {
		if text := strings.TrimSpace(line); text != "" {
			add(text, white)
		}
	}
	return lines
}

// These native item families use ConstHP/SP/MP as restoration amounts, not
// equipment max-pool bonuses (0x4a3950). Their descriptions carry that effect.
func itemHasRestoration(typ uint8) bool {
	switch typ {
	case 25, 44, 38, 41, 78, 79, 80, 100, 101, 102, 103:
		return true
	}
	return false
}

var defaultWeaponEnchantAttack = [21]uint32{0, 7, 14, 21, 31, 41, 51, 64, 77, 90, 106, 122, 138, 157, 176, 195, 217, 239, 261, 286, 311}

func itemEnchantEligibility(def *sdata.Item, instance *proto.InventoryItem) (craftValues, byte, bool) {
	rt := itemtype.Real(def.Type)
	if rt < 1 || rt > 25 || itemHasRestoration(def.Type) || instance == nil || def.Server == 0 || def.RequiredStats[4] == 0 || def.RequiredStats[4] >= 100 {
		return craftValues{}, 0, false
	}
	craft, ok := parseCraftValues(instance.Craft)
	if !ok || craft.enchant == 0 {
		return craftValues{}, 0, false
	}
	raw := strings.TrimRight(instance.Craft, " ")
	return craft, raw[18], true
}

func itemWeaponEnchantAttack(def *sdata.Item, instance *proto.InventoryItem, rules ...*proto.EnchantRules) uint32 {
	if rt := itemtype.Real(def.Type); rt < 1 || rt > 15 {
		return 0
	}
	craft, highDigit, ok := itemEnchantEligibility(def, instance)
	if !ok || highDigit >= '5' {
		return 0
	}
	if len(rules) > 0 && rules[0] != nil {
		return rules[0].WeaponBonus[craft.enchant]
	}
	if int(craft.enchant) < len(defaultWeaponEnchantAttack) {
		return defaultWeaponEnchantAttack[craft.enchant]
	}
	return 0
}

// itemAbsorption combines native Exp and eligible armor enchantment. Linked
// lapis are added by drawItemDetails; the craft level remains in the item name.
func itemAbsorption(def *sdata.Item, instance *proto.InventoryItem, rules ...*proto.EnchantRules) uint32 {
	total := uint32(def.ExpAbsorb)
	craft, highDigit, ok := itemEnchantEligibility(def, instance)
	if !ok || highDigit < '5' {
		return total
	}
	if len(rules) > 0 && rules[0] != nil {
		total += rules[0].ArmorBonus[craft.enchant]
	} else if craft.enchant <= 20 {
		total += uint32(craft.enchant) * 5
	}
	return total
}
