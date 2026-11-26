package scene

import (
	"slices"

	"github.com/Spelunkern/Shaiya-GX/internal/render"
)

// PreservePosture transfers the active animation state to a rebuilt body.
// Swapping equipment or appearance on the same body preserves locomotion,
// posture, mounted clips, actions, casts, blends, playback rate, and
// frame-event cursors instead of restarting from idle.
func (c *Character) PreservePosture(previous *Character) {
	if c == nil || previous == nil || previous.clip == nil || c.prefix != previous.prefix || c.folder != previous.folder {
		return
	}
	c.clip, c.clipName, c.clipFile = previous.clip, previous.clipName, previous.clipFile
	c.Time, c.oneShot, c.locked, c.hold = previous.Time, previous.oneShot, previous.locked, previous.hold
	c.actions, c.actionsLoaded, c.actionSlot, c.actionID = previous.actions, previous.actionsLoaded, previous.actionSlot, previous.actionID
	c.reactionID, c.combatPoseLeft = previous.reactionID, previous.combatPoseLeft
	c.castLeft, c.castTotal, c.castLoop = previous.castLeft, previous.castTotal, previous.castLoop
	c.spinLeft, c.externalTime, c.hiddenWeaponAction = previous.spinLeft, previous.externalTime, previous.hiddenWeaponAction
	c.eventAction, c.eventPrev, c.eventCur, c.eventClip = previous.eventAction, previous.eventPrev, previous.eventCur, previous.eventClip
	c.rate = previous.rate
	c.blendLeft, c.blendToFirstFrame, c.locomotionBlend = previous.blendLeft, previous.blendToFirstFrame, previous.locomotionBlend
	c.blendFrom = slices.Clone(previous.blendFrom)
	c.enchantTime = previous.enchantTime
	c.Mounted = previous.Mounted
	if c.cloak != nil && previous.cloak != nil {
		c.cloak.elapsed = previous.cloak.elapsed
	}
	c.invalidatePose()
}

// The original character and creature controllers blend for five frames at 30 Hz.
const clipBlendDuration = float32(5.0 / 30)

type clipBlendMode uint8

const (
	blendMatchingSkeletons clipBlendMode = iota
	blendSharedBones
	blendToFirstKey
)

func (c *Character) invalidatePose() {
	c.lastFinalsClip = nil
	c.poseReady = false
	c.boundReady = false
	c.shadowFitReady = false
}

func (c *Character) startClip(next *Clip, oneShot bool) {
	c.startClipWithBlend(next, oneShot, blendMatchingSkeletons)
}

func (c *Character) startClipWithBlend(next *Clip, oneShot bool, mode clipBlendMode) {
	previous := c.clip
	nextCount := len(next.ani.Bones)
	count := 0
	compatible := false
	if previous != nil {
		previousCount := len(previous.ani.Bones)
		count = min(previousCount, nextCount)
		compatible = count > 0 && (previousCount == nextCount || mode != blendMatchingSkeletons)
	}
	if compatible {
		for i, bone := range next.ani.Bones[:count] {
			if bone.Parent != previous.ani.Bones[i].Parent {
				compatible = false
				break
			}
		}
	}
	if compatible {
		from := c.blendFrom
		if cap(from) < count {
			from = make([]bonePose, count)
		} else {
			from = from[:count]
		}
		frame := previous.frame(c.Time, c.oneShot)
		for i := range from {
			// If interrupted, continue from the visible blend rather than
			// snapping back to either of the two earlier clips.
			from[i] = c.localPose(i, frame)
		}
		c.blendFrom = from
		c.blendLeft = clipBlendDuration
	} else {
		c.blendLeft = 0
	}
	c.resetClip(next, oneShot)
	c.blendToFirstFrame = compatible && mode == blendToFirstKey
}

func (c *Character) startClipImmediately(next *Clip, oneShot bool) {
	c.blendLeft = 0
	c.resetClip(next, oneShot)
}

func (c *Character) resetClip(next *Clip, oneShot bool) {
	c.clip, c.Time, c.oneShot = next, 0, oneShot
	c.locomotionBlend = false
	c.blendToFirstFrame = false
	c.rate.kind, c.rate.next = c.rate.next, rateNone
	c.invalidatePose()
}

func (c *Character) advanceBlend(dt float32) float32 {
	if c.blendLeft <= 0 || dt <= 0 {
		return dt
	}
	step := min(c.blendLeft, dt)
	c.blendLeft -= step
	c.invalidatePose()
	return dt - step
}

func (c *Character) localPose(i int, frame float32) bonePose {
	if c.blendLeft <= 0 || i >= len(c.blendFrom) {
		return c.clip.localPose(i, frame)
	}
	from := c.blendFrom[i]
	t := 1 - c.blendLeft/clipBlendDuration
	if t == 0 {
		return from
	}
	var into bonePose
	if c.blendToFirstFrame {
		into = c.clip.localPose(i, float32(c.clip.ani.Start))
	} else {
		into = c.clip.restPose(i)
	}
	into.rotation = render.Slerp(from.rotation, into.rotation, t)
	for axis, value := range from.position {
		into.position[axis] = value + (into.position[axis]-value)*t
	}
	return into
}
