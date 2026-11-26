package app

import (
	"testing"

	"github.com/Spelunkern/Shaiya-GX/internal/proto"
)

// Game.exe 0x4c9140: weapons, armor, accessories and the rest each map to one
// ui_armorNNN cue; type 0 and unknown types are mute.
func TestItemSoundNames(t *testing.T) {
	for _, tc := range []struct {
		itemType uint8
		want     string
	}{
		{1, "ui_armor014.ogg"}, {45, "ui_armor014.ogg"}, {16, "ui_armor002.ogg"}, {31, "ui_armor002.ogg"},
		{26, "ui_armor024.ogg"}, {97, "ui_armor021.ogg"}, {0, ""}, {98, ""}, {255, ""},
	} {
		if got := itemSoundName(tc.itemType); got != tc.want {
			t.Fatalf("type %d: got %q want %q", tc.itemType, got, tc.want)
		}
	}
}

func TestGroundDropSound(t *testing.T) {
	for _, tc := range []struct {
		itemType uint8
		want     string
	}{{26, "ui_moneydrop1.ogg"}, {22, "ui_itemdrop2.ogg"}, {100, "ui_itemdrop2.ogg"}, {1, "ui_itemdrop1.ogg"}, {31, "ui_itemdrop1.ogg"}, {98, "ui_itemdrop1.ogg"}} {
		if got := groundDropSound(tc.itemType); got != tc.want {
			t.Fatalf("type %d: %q want %q", tc.itemType, got, tc.want)
		}
	}
}

func TestEquipmentMoveSoundType(t *testing.T) {
	weapon := proto.InventoryItem{Bag: 1, Slot: 2, Type: 1, TypeID: 7, Count: 1}
	equipped := weapon
	equipped.Bag, equipped.Slot = 0, 5
	shield := proto.InventoryItem{Bag: 0, Slot: 6, Type: 15, TypeID: 8, Count: 1}
	replacement := equipped
	replacement.TypeID = 8
	emptyBag := proto.InventoryItem{Bag: 1, Slot: 2}
	emptyEquip := proto.InventoryItem{Bag: 0, Slot: 5}
	otherBag := weapon
	otherBag.Bag = 2
	for _, tc := range []struct {
		name                string
		before              []proto.InventoryItem
		source, destination proto.InventoryItem
		want                uint8
	}{
		{"equip", []proto.InventoryItem{weapon}, emptyBag, equipped, weapon.Type},
		{"unequip", []proto.InventoryItem{equipped}, emptyEquip, weapon, weapon.Type},
		{"replace", []proto.InventoryItem{weapon, replacement}, weapon, equipped, weapon.Type},
		{"rejected equip", []proto.InventoryItem{weapon, replacement}, weapon, replacement, 0},
		{"rejected unequip", []proto.InventoryItem{shield}, shield, emptyBag, 0},
		{"bag rearrangement", []proto.InventoryItem{weapon}, emptyBag, otherBag, 0},
		{"same slot refresh", []proto.InventoryItem{replacement}, equipped, equipped, 0},
		{"duplicate equip reply", []proto.InventoryItem{equipped}, emptyBag, equipped, 0},
		{"swap identical equipped items", []proto.InventoryItem{
			{Bag: 1, Slot: 2, Type: 17, TypeID: 12, Quality: 100, Count: 1},
			{Bag: 0, Slot: 1, Type: 17, TypeID: 12, Quality: 100, Count: 1},
		}, proto.InventoryItem{Bag: 1, Slot: 2, Type: 17, TypeID: 12, Quality: 100, Count: 1}, proto.InventoryItem{Bag: 0, Slot: 1, Type: 17, TypeID: 12, Quality: 100, Count: 1}, 17},
		{"swap identical equipment slots", []proto.InventoryItem{
			{Bag: 0, Slot: 9, Type: 22, TypeID: 5, Count: 1},
			{Bag: 0, Slot: 10, Type: 22, TypeID: 5, Count: 1},
		}, proto.InventoryItem{Bag: 0, Slot: 9, Type: 22, TypeID: 5, Count: 1}, proto.InventoryItem{Bag: 0, Slot: 10, Type: 22, TypeID: 5, Count: 1}, 22},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := inputApp()
			for _, item := range tc.before {
				a.state.Inventory[[2]uint8{item.Bag, item.Slot}] = item
			}
			w := proto.NewWriter(proto.ItemBagToBag)
			for _, item := range []proto.InventoryItem{tc.source, tc.destination} {
				w.Raw([]byte{item.Bag, item.Slot, item.Type, item.TypeID})
				w.Count(item.Count)
				w.U16(item.Quality)
				w.Raw(item.Gems[:])
				w.Fixed(item.Craft, 21, true)
			}
			if got := a.equipmentMoveSoundType(w.Bytes()); got != tc.want {
				t.Fatalf("equipment sound type = %d, want %d", got, tc.want)
			}
		})
	}
}
