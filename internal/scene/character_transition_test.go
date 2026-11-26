package scene

import (
	"math"
	"testing"

	"github.com/Spelunkern/Shaiya-GX/internal/data/model"
	"github.com/Spelunkern/Shaiya-GX/internal/render"
)

func TestClipTransitionPreservesPoseAndDelaysPlayback(t *testing.T) {
	outgoing := poseTestClip(2)
	incoming := poseTestClip(2)
	incoming.restLocal[0][12] = 2
	incoming.restLocal[1][12] = 3
	for i := range incoming.ani.Bones {
		incoming.ani.Bones[i].Trans = nil
	}
	c := &Character{clip: outgoing, Time: .5}
	last := c.finals()[1]
	c.startClip(incoming, false)
	if c.finals()[1] != last {
		t.Fatal("clip switched before the transition")
	}
	c.Update(clipBlendDuration / 2)
	middle := c.finals()[1]
	if c.Time != 0 || math.Abs(float64(middle[12]-(last[12]+5)/2)) > .0001 {
		t.Fatal("blend did not interpolate local pose while holding playback", c.Time, middle)
	}
	c.startClip(outgoing, false)
	if c.finals()[1] != middle {
		t.Fatal("interruption discarded the pose currently shown")
	}
	c.Update(clipBlendDuration)
	if c.Time != 0 || c.blendLeft != 0 || c.finals()[1] != outgoing.Finals(0)[1] {
		t.Fatal("incoming playback did not begin at its opening pose")
	}
	c.Update(.1)
	if c.Time != .1 {
		t.Fatal("incoming playback failed to advance")
	}
}

func TestClipTransitionHoldsFinalActionPose(t *testing.T) {
	clip := poseTestClip(1)
	c := &Character{clip: clip, locked: true, oneShot: true, Time: .95}
	c.Update(.1)
	if c.ActionActive() || c.Time != 1 || c.finals()[0][13] != .5 {
		t.Fatal("completed action wrapped to its opening pose", c.Time, c.finals())
	}
	c.startClip(poseTestClip(1), false)
	if c.finals()[0][13] != .5 {
		t.Fatal("the outgoing transition did not start at the last action frame")
	}
}

func TestClipTransitionFallsBackForDifferentSkeletons(t *testing.T) {
	first := poseTestClip(2)
	second := poseTestClip(2)
	second.ani.Bones[1].Parent = -1
	c := &Character{clip: first}
	c.startClip(second, false)
	if c.blendLeft != 0 || c.finals()[1] != second.Finals(0)[1] {
		t.Fatal("blended incompatible hierarchies")
	}
}

func TestPlayerLocomotionBlendsSharedBones(t *testing.T) {
	assets := NewAssets(nil)
	run, idle := poseTestClip(3), poseTestClip(2)
	assets.clips["ani:character/human/ani/humf_040_onrun.ani"] = run
	assets.clips["ani:character/human/ani/humf_000_normal.ani"] = idle
	assets.clips["ani:character/human/ani/humf_034_onready.ani"] = idle
	assets.clips["ani:character/human/ani/humf_001_walk.ani"] = idle
	assets.clips["ani:character/human/ani/humf_018_ladder.ani"] = idle
	actions := make([]model.Action, 41)
	actions[40].Name = "humf_040_onrun.ANI"
	actions[34].Name = "humf_034_onready.ANI"

	for _, tc := range []struct {
		name, destination string
		combatPose        bool
	}{
		{name: "normal idle", destination: "000_normal"},
		{name: "weapon ready", destination: "000_normal", combatPose: true},
		{name: "walk", destination: "001_walk"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &Character{assets: assets, folder: "Human", prefix: "humf", weaponType: 45, actionsLoaded: true, actions: actions}
			c.SetClip("002_run")
			c.Update(.5)
			last := c.finals()[1]
			if tc.combatPose {
				c.combatPoseLeft = 10
			}
			c.SetClip(tc.destination)
			if c.blendLeft != clipBlendDuration || c.finals()[1] != last {
				t.Fatal("locomotion changed without preserving the shared bones")
			}
			c.Update(clipBlendDuration / 2)
			action := c.ActionID()
			c.SetClip("002_run")
			if c.Time != 0 || c.blendLeft != clipBlendDuration/2 || c.finals()[1] == last || c.ActionID() != action {
				t.Fatal("a rapid input change interrupted locomotion blending")
			}
			c.Update(clipBlendDuration / 2)
			if c.blendLeft != 0 || c.finals()[1] != idle.Finals(0)[1] {
				t.Fatal("locomotion did not reach the destination pose")
			}
			c.SetClip("002_run")
			if c.blendLeft != clipBlendDuration || c.finals()[1] != idle.Finals(0)[1] {
				t.Fatal("held input did not blend into running")
			}
			c.SetClip("018_ladder")
			if !c.PlayingClip("018_ladder") || c.blendLeft != 0 || c.Time != 0 {
				t.Fatal("ladder entry did not activate immediately")
			}
		})
	}
}

func TestJumpStartsImmediatelyAndReturnsThroughNormalIdle(t *testing.T) {
	assets := NewAssets(nil)
	run, jump, idle := poseTestClip(3), poseTestClip(3), poseTestClip(2)
	run.restLocal[1][12] = 2
	run.ani.Bones[1].Trans[0].T[0] = 1
	assets.clips["ani:character/human/ani/humf_002_run.ani"] = run
	assets.clips["ani:character/human/ani/humf_008_jump.ani"] = jump
	assets.clips["ani:character/human/ani/humf_000_normal.ani"] = idle
	actions := make([]model.Action, 35)
	actions[8] = model.Action{Name: "humf_008_jump.ANI", Mode: 2}
	actions[34].Name = "humf_034_onready.ANI"
	c := &Character{assets: assets, folder: "Human", prefix: "humf", weaponType: 45, actionsLoaded: true, actions: actions}
	c.startClip(poseTestClip(3), false)
	c.SetClip("002_run")
	if c.blendLeft != clipBlendDuration {
		t.Fatal("fixture must start in a locomotion blend")
	}
	if !c.PlayAction(8, false) || c.blendLeft != 0 || c.finals()[1] != jump.Finals(0)[1] {
		t.Fatal("jump did not immediately replace the locomotion blend")
	}
	c.Update(.125)
	if c.Time != .125 {
		t.Fatalf("jump playback was delayed: cursor %v", c.Time)
	}
	jumpEnd := &Character{clip: jump, oneShot: true, Time: jump.Duration()}
	c.combatPoseLeft = 10
	c.Update(jump.Duration() - c.Time)
	if !c.PlayingClip("000_normal") || c.blendLeft != clipBlendDuration || c.finals()[1] != jumpEnd.finals()[1] {
		t.Fatal("jump did not blend from its last pose to normal idle")
	}
	c.SetClip("002_run")
	c.Update(clipBlendDuration / 2)
	c.SetClip("000_normal")
	if !c.PlayingClip("000_normal") || c.Time != 0 || c.finals()[1] == jumpEnd.finals()[1] {
		t.Fatal("locomotion or weapon-ready input interrupted the landing blend")
	}
	c.Update(clipBlendDuration / 2)
	if c.finals()[1] != idle.Finals(0)[1] {
		t.Fatal("landing blend did not reach the opening idle pose")
	}
	lastIdle, firstRun := c.finals()[1], run.Finals(0)[1]
	c.SetClip("002_run")
	if !c.PlayingClip("002_run") || c.blendLeft != clipBlendDuration || c.finals()[1] != lastIdle {
		t.Fatal("resuming locomotion dropped the pose of the shared bones")
	}
	c.Update(clipBlendDuration / 2)
	if c.Time != 0 || math.Abs(float64(c.finals()[1][12]-(lastIdle[12]+firstRun[12])/2)) > .0001 {
		t.Fatal("resuming locomotion did not blend toward its first keyed pose")
	}
	c.Update(clipBlendDuration / 2)
	if c.finals()[1] != firstRun || c.blendLeft != 0 {
		t.Fatal("resuming locomotion did not finish at the first keyed pose")
	}

	c.PlayAction(8, false)
	c.Update(jump.Duration())
	c.Update(clipBlendDuration)
	c.SetClip("002_run")
	c.Update(clipBlendDuration / 2)
	extraBone := c.finals()[2]
	c.startClip(poseTestClip(3), true)
	if c.finals()[2] != extraBone {
		t.Fatal("interrupting a locomotion blend dropped an incoming bone's pose")
	}

	c.PlayAction(8, false)
	c.Update(jump.Duration())
	c.Update(clipBlendDuration)
	c.combatPoseLeft = 0
	c.SetClip("000_normal")
	c.SetClip("002_run")
	if c.blendLeft != clipBlendDuration || c.finals()[1] != idle.Finals(0)[1] {
		t.Fatal("ordinary running from idle also needs to blend the shared bones")
	}
}

func TestClipTransitionUsesLocalQuaternionInterpolation(t *testing.T) {
	first := poseTestClip(2)
	second := poseTestClip(2)
	second.ani.Bones[0].Rot, second.ani.Bones[0].Trans = nil, nil
	second.restLocal[0] = render.QuatMatrix([4]float32{0, 0, 1, 0}, [3]float32{})
	c := &Character{clip: first, Time: .5}
	c.startClip(second, false)
	c.Update(clipBlendDuration / 2)
	// A spherical half-turn blend reaches a quarter-turn matrix. Direct
	// matrix interpolation would collapse the two rotation axes here.
	m := c.finals()[0]
	if math.Abs(float64(m[0])) > .0001 || math.Abs(math.Abs(float64(m[1]))-1) > .0001 {
		t.Fatal("blend did not keep a rigid rotation", m)
	}
}

func TestIdleVariationBlendsBothDirectionsAndYieldsToMovement(t *testing.T) {
	assets := NewAssets(nil)
	rest, variation, run := poseTestClip(2), poseTestClip(3), poseTestClip(3)
	assets.clips["ani:character/human/ani/humf_000_normal.ani"] = rest
	assets.clips["ani:character/human/ani/humf_016_idle1.ani"] = variation
	assets.clips["ani:character/human/ani/humf_017_idle2.ani"] = variation
	assets.clips["ani:character/human/ani/humf_002_run.ani"] = run
	actions := make([]model.Action, 18)
	actions[16] = model.Action{Name: "humf_016_idle1.ANI", Mode: 2}
	actions[17] = model.Action{Name: "humf_017_idle2.ANI", Mode: 2}
	c := &Character{assets: assets, folder: "Human", prefix: "humf", actions: actions, actionsLoaded: true}
	c.SetClip("000_normal")
	c.Update(.25)
	previous := c.finals()[1]
	if !c.PlayIdleVariation(16) || c.blendLeft != clipBlendDuration || c.finals()[1] != previous {
		t.Fatal("idle variation snapped from the previous pose")
	}
	c.Update(clipBlendDuration)
	if c.Time != 0 || !c.PlayingIdleVariation() || c.PlayIdleVariation(17) {
		t.Fatal("variation missed its first frame or was restarted during playback")
	}
	c.Update(variation.Duration())
	final := (&Character{clip: variation, Time: variation.Duration(), oneShot: true}).finals()[1]
	if !c.PlayingLocomotionClip("000_normal") || c.blendLeft != clipBlendDuration || c.finals()[1] != final {
		t.Fatal("variation did not blend from its final frame into normal idle")
	}
	c.Update(clipBlendDuration)
	if !c.PlayIdleVariation(17) {
		t.Fatal("second native variation could not play")
	}
	c.Update(.05)
	visible := c.finals()[1]
	c.SetClip("002_run")
	if !c.PlayingLocomotionClip("002_run") || c.finals()[1] != visible {
		t.Fatal("running did not interrupt the variation from the visible pose")
	}
}

func TestPreservePostureKeepsAnimationAcrossEquipmentRebuild(t *testing.T) {
	assets := NewAssets(nil)
	idle := poseTestClip(2)
	walk := poseTestClip(2)
	run := poseTestClip(2)
	swordRun := poseTestClip(2)
	assets.clips["ani:character/human/ani/humf_000_normal.ani"] = idle
	assets.clips["ani:character/human/ani/humf_001_walk.ani"] = walk
	assets.clips["ani:character/human/ani/humf_002_run.ani"] = run
	assets.clips["ani:character/human/ani/humf_040_onrun.ani"] = swordRun
	actions := make([]model.Action, 41)
	actions[40].Name = "humf_040_onrun.ANI"
	actions[10] = model.Action{Name: "humf_001_walk.ANI", Mode: 2}

	t.Run("locomotion keeps clip, time and move rate", func(t *testing.T) {
		for _, clipName := range []string{"001_walk", "002_run"} {
			prev := &Character{assets: assets, folder: "Human", prefix: "humf", actionsLoaded: true, actions: actions}
			prev.SetSpeedTiers(5, 5, 0, false)
			prev.SetClip(clipName)
			prev.Update(.35)

			rebuilt := &Character{assets: assets, folder: "Human", prefix: "humf", actionsLoaded: true, actions: actions}
			rebuilt.SetSpeedTiers(5, 5, 0, false)
			rebuilt.SetClip("000_normal")
			rebuilt.PreservePosture(prev)

			if rebuilt.clip != prev.clip || rebuilt.clipFile != prev.clipFile || rebuilt.Time != prev.Time {
				t.Fatalf("%s: clip/time not preserved: got (%v, %q, %v) want (%v, %q, %v)",
					clipName, rebuilt.clip, rebuilt.clipFile, rebuilt.Time, prev.clip, prev.clipFile, prev.Time)
			}
			if rebuilt.rate.kind != rateMove || rebuilt.eventPrev != prev.eventPrev || rebuilt.eventCur != prev.eventCur {
				t.Fatalf("%s: rate/event cursor not preserved", clipName)
			}
			beforeTime := rebuilt.Time
			rebuilt.SetClip(clipName)
			if rebuilt.Time != beforeTime || rebuilt.blendLeft != 0 {
				t.Fatalf("%s: SetClip restarted animation after rebuild: time=%v->%v blend=%v",
					clipName, beforeTime, rebuilt.Time, rebuilt.blendLeft)
			}
		}
	})

	t.Run("active action, cast and blend are preserved", func(t *testing.T) {
		prev := &Character{assets: assets, folder: "Human", prefix: "humf", actionsLoaded: true, actions: actions}
		prev.SetClip("000_normal")
		if !prev.PlayCast(10, 1.5) {
			t.Fatal("failed to start cast action")
		}
		prev.Update(clipBlendDuration / 2)

		rebuilt := &Character{assets: assets, folder: "Human", prefix: "humf"}
		rebuilt.SetClip("000_normal")
		rebuilt.PreservePosture(prev)

		if !rebuilt.PlayingAction(10) || rebuilt.castLeft != prev.castLeft || rebuilt.blendLeft != prev.blendLeft || len(rebuilt.blendFrom) != len(prev.blendFrom) {
			t.Fatal("active action/cast/blend state was lost on rebuild")
		}
	})

	t.Run("weapon swap blends smoothly into new run clip", func(t *testing.T) {
		prev := &Character{assets: assets, folder: "Human", prefix: "humf", actionsLoaded: true, actions: actions}
		prev.SetClip("002_run")
		prev.Update(.3)
		last := prev.finals()[1]

		rebuilt := &Character{assets: assets, folder: "Human", prefix: "humf", weaponType: 1, actionsLoaded: true, actions: actions}
		rebuilt.SetClip("000_normal")
		rebuilt.PreservePosture(prev)
		rebuilt.SetClip("002_run")

		if rebuilt.clip != swordRun || rebuilt.blendLeft != clipBlendDuration || rebuilt.finals()[1] != last {
			t.Fatal("weapon change did not blend from previous run pose into new weapon run clip")
		}
	})

	t.Run("incompatible body does not copy clip", func(t *testing.T) {
		prev := &Character{assets: assets, folder: "Human", prefix: "humf"}
		prev.SetClip("001_walk")
		prev.Update(.2)

		disguise := &Character{assets: assets, folder: "Elf", prefix: "elf"}
		disguise.clip = idle
		disguise.PreservePosture(prev)
		if disguise.clip != idle || disguise.Time != 0 {
			t.Fatal("incompatible body copied previous clip")
		}
	})
}
