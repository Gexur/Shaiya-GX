package render

import (
	"testing"

	"github.com/Spelunkern/Shaiya-GX/internal/render/gl"
)

func TestEffectRunsFragmentedThreshold(t *testing.T) {
	for _, c := range []struct {
		runs, commands int
		want           bool
	}{
		{16, 20, false},  // few runs: keep the multi-draw
		{17, 136, false}, // eight commands per run on average
		{17, 135, true},  // just under eight per run
		{100, 120, true}, // hundreds of tiny runs
		{3, 300, false},
	} {
		if got := effectRunsFragmented(c.runs, c.commands); got != c.want {
			t.Fatalf("runs=%d commands=%d: %v", c.runs, c.commands, got)
		}
	}
}

func TestEffectPremulMergesBlendRuns(t *testing.T) {
	m := &Mesh{Count: 6, indexType: gl.UNSIGNED_SHORT, indexBytes: 2, immutable: true, vbo: 1, bytes: 256}
	tex := &Texture{ID: 1, samplingViews: [4]textureSamplingView{{id: 21, handle: 2001}, {id: 22, handle: 2002}}}
	p := effectIndirect{
		program:  1,
		bound:    true,
		geometry: map[*Mesh]effectIndirectLocation{m: {vertex: 5, index: 0}},
	}
	draws := []queuedEffect{
		{mesh: m, texture: tex, src: gl.SRC_ALPHA, dst: gl.ONE_MINUS_SRC_ALPHA, first: 0, end: 1},
		{mesh: m, texture: tex, src: gl.SRC_ALPHA, dst: gl.ONE, first: 1, end: 2},
		{mesh: m, texture: tex, src: gl.ONE, dst: gl.ONE, first: 2, end: 3},
		{mesh: m, texture: tex, src: gl.SRC_ALPHA, dst: gl.ONE_MINUS_SRC_ALPHA, first: 3, end: 4},
		{mesh: m, texture: tex, src: gl.DST_COLOR, dst: gl.ZERO, first: 4, end: 5},
	}
	if !p.plan(draws) {
		t.Fatal("plan() returned false")
	}
	if len(p.runs) != 2 {
		t.Fatalf("runs = %d, want 2", len(p.runs))
	}
	if p.runs[0].src != gl.ONE || p.runs[0].dst != gl.ONE_MINUS_SRC_ALPHA || p.runs[0].count != 4 {
		t.Fatalf("run[0] = %+v, want premul ONE/ONE_MINUS_SRC_ALPHA count=4", p.runs[0])
	}
	if p.runs[1].src != gl.DST_COLOR || p.runs[1].dst != gl.ZERO || p.runs[1].count != 1 {
		t.Fatalf("run[1] = %+v, want DST_COLOR/ZERO count=1", p.runs[1])
	}
	for i, wantMode := range []uint32{1, 2, 3, 1, 0} {
		if gotMode := p.materials[i][3] >> 30; gotMode != wantMode {
			t.Fatalf("material[%d] mode = %d, want %d", i, gotMode, wantMode)
		}
	}
}
