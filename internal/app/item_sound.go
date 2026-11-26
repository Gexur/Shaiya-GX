package app

import (
	"fmt"

	"github.com/Spelunkern/Shaiya-GX/internal/proto"
)

// itemSoundTable is Game.exe 0x4c9140: the ui_armorNNN cue of every item type
// (1..97). GX plays it when a confirmed move changes equipment.
var itemSoundTable = [98]uint8{0, 14, 15, 12, 13, 6, 8, 10, 11, 16, 16, 8, 7, 17, 18, 9, 2, 0, 1, 5, 3, 4, 19, 20, 22, 23, 24, 23, 23, 23, 23, 2, 0, 1, 5, 3, 4, 19, 23, 22, 21, 23, 23, 23, 23, 14, 15, 12, 13, 6, 6, 8, 8, 10, 10, 11, 11, 16, 16, 8, 7, 7, 17, 17, 18, 9, 2, 0, 1, 5, 3, 4, 2, 0, 1, 5, 3, 4, 23, 23, 23, 2, 0, 1, 5, 3, 4, 2, 0, 1, 5, 3, 4, 23, 19, 23, 20, 21}

func itemSoundName(itemType uint8) string {
	if itemType == 0 || int(itemType) >= len(itemSoundTable) {
		return ""
	}
	return fmt.Sprintf("ui_armor%03d.ogg", itemSoundTable[itemType])
}

func sameItemValue(a, b proto.InventoryItem) bool {
	if a.Type == 0 || a.Count == 0 {
		return false
	}
	a.Bag, a.Slot = 0, 0
	b.Bag, b.Slot = 0, 0
	return a == b
}

// Compare the confirmed slots with inventory before applying the packet.
// World also echoes both slots unchanged when it refuses a move.
func (a *App) equipmentMoveSoundType(p []byte) uint8 {
	moved, err := proto.DecodeItemMove(p)
	if err != nil || moved[0].Bag == moved[1].Bag && moved[0].Slot == moved[1].Slot {
		return 0
	}
	// Prefer the destination equipment slot when a move changes both.
	for _, pair := range [2][2]proto.InventoryItem{{moved[1], moved[0]}, {moved[0], moved[1]}} {
		after, other := pair[0], pair[1]
		if after.Bag != 0 {
			continue
		}
		before := a.state.Inventory[[2]uint8{after.Bag, after.Slot}]
		if after.Type == 0 || after.Count == 0 {
			if before.Type != 0 {
				return before.Type
			}
		} else if after != before || sameItemValue(after, other) {
			return after.Type
		}
	}
	return 0
}

// playItemSound is a global interface cue (native position 0x22094c8, no
// distance falloff).
func (a *App) playItemSound(itemType uint8) {
	if a.sound == nil {
		return
	}
	if name := itemSoundName(itemType); name != "" {
		file := a.audioPath(name, false)
		a.audioError(file, a.sound.Play(file, .6))
	}
}

// groundDropSound is Game.exe 0x428470: a dropped item announces itself with a
// cue chosen by its type. Money has its own; types 22-25, 27-30, 37-41, 44,
// 78-80, 96-97 and 99-103 use ui_itemdrop2 and the rest ui_itemdrop1.
func groundDropSound(itemType uint8) string {
	switch itemType {
	case 26:
		return "ui_moneydrop1.ogg"
	case 22, 23, 24, 25, 27, 28, 29, 30, 37, 38, 39, 40, 41, 44, 78, 79, 80, 96, 97, 99, 100, 101, 102, 103:
		return "ui_itemdrop2.ogg"
	}
	return "ui_itemdrop1.ogg"
}
