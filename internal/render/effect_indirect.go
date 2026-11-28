package render

import (
	"cmp"
	"unsafe"

	"github.com/Spelunkern/Shaiya-GX/internal/render/gl"
)

const (
	defaultEffectMaxMeshes   = 2048
	defaultEffectMaxVertices = 32 << 20
	defaultEffectMaxIndices  = 16 << 20
	defaultEffectMaxKeys     = 64 << 20
)

type effectIndirectLocation struct {
	vertex, index int
	keys          uint32 // zero for static geometry, otherwise first key vertex + 1
}

type effectIndirectRun struct {
	first, count        int
	serial              int
	indexType, src, dst uint32
	textures            drawTextures
}

type effectIndirect struct {
	program, vao                            uint32
	bound, materialAttribute                bool
	boundVertexCapacity, boundIndexCapacity int
	maxMeshes, maxVertices                  int
	maxIndices, maxKeys                     int
	vertices, indices, keys                 packedMeshBuffer
	geometry                                map[*Mesh]effectIndirectLocation
	commands                                []indexedDrawCommand
	materials                               [][4]uint32
	runs                                    []effectIndirectRun
	commandData, drawData                   effectUpload
	reclaimDead                             bool
}

func (p *effectIndirect) limits() (meshes, vertices, indices, keys int) {
	return cmp.Or(p.maxMeshes, defaultEffectMaxMeshes),
		cmp.Or(p.maxVertices, defaultEffectMaxVertices),
		cmp.Or(p.maxIndices, defaultEffectMaxIndices),
		cmp.Or(p.maxKeys, defaultEffectMaxKeys)
}

func (p *effectIndirect) meshLocation(m *Mesh, g *EffectGeometry) (effectIndirectLocation, bool) {
	if !m.immutable || m.vbo == 0 {
		return effectIndirectLocation{}, false
	}
	if pos, ok := p.geometry[m]; ok {
		return pos, true
	}
	maxMeshes, maxVertices, maxIndices, maxKeys := p.limits()
	const keyStride = int(unsafe.Sizeof(EffectKeyVertex{}))
	indexBytes := int(m.Count) * int(m.indexBytes)
	vertexBytes := int(m.bytes) - indexBytes
	firstIndex := (p.indices.used + 3) &^ 3
	keyBytes := 0
	if g != nil {
		keyBytes = int(g.bytes)
	}
	if len(p.geometry) == maxMeshes || vertexBytes <= 0 || indexBytes <= 0 ||
		p.vertices.used+vertexBytes > maxVertices || firstIndex+indexBytes > maxIndices || p.keys.used+keyBytes > maxKeys {
		return effectIndirectLocation{}, false
	}
	pos := effectIndirectLocation{vertex: p.vertices.used / vertexSize, index: firstIndex}
	p.vertices.copy(m.vbo, p.vertices.used, vertexBytes, maxVertices)
	p.indices.copy(m.ebo, firstIndex, indexBytes, maxIndices)
	if keyBytes != 0 {
		pos.keys = uint32(p.keys.used/keyStride + 1)
		p.keys.copy(g.keys, p.keys.used, keyBytes, maxKeys)
	}
	if p.geometry == nil {
		p.geometry = make(map[*Mesh]effectIndirectLocation)
	}
	p.geometry[m] = pos
	return pos, true
}

func (p *effectIndirect) plan(draws []queuedEffect) bool {
	if p.program == 0 || len(draws) < 4 {
		return false
	}
	p.commands, p.materials, p.runs = p.commands[:0], p.materials[:0], p.runs[:0]
	p.reclaimDead = false
	for i, draw := range draws {
		loc, ok := p.meshLocation(draw.mesh, draw.geometry)
		if !ok {
			// A closed local arena or streamed map may leave entries in the
			// bounded cache. Reclaim them once this pass finishes.
			if !p.reclaimDead {
				for m := range p.geometry {
					if m.vbo == 0 {
						p.reclaimDead = true
						break
					}
				}
			}
			p.newSerialRun(i)
			continue
		}
		var view *textureSamplingView
		var textureRef uint64
		if p.bound {
			view = draw.texture.samplingView(draw.repeat)
			if view == nil {
				p.newSerialRun(i)
				continue
			}
		} else {
			textureRef = draw.texture.bindlessHandle(draw.repeat)
			if textureRef == 0 {
				p.newSerialRun(i)
				continue
			}
		}
		indexType := draw.mesh.indexType
		if n := len(p.runs); n == 0 || p.runs[n-1].serial >= 0 || p.runs[n-1].indexType != indexType || p.runs[n-1].src != draw.src || p.runs[n-1].dst != draw.dst {
			p.newRun(draw)
		}
		if p.bound {
			slot, ok := p.runs[len(p.runs)-1].textures.slot(view.id, drawTextureLimit)
			if !ok {
				p.newRun(draw)
				slot, _ = p.runs[len(p.runs)-1].textures.slot(view.id, drawTextureLimit)
			}
			textureRef = uint64(slot)
		}
		command := indexedDrawCommand{uint32(draw.mesh.Count), uint32(draw.end - draw.first), uint32(loc.index / int(draw.mesh.indexBytes)), uint32(loc.vertex), uint32(draw.first)}
		material := [4]uint32{uint32(textureRef), uint32(textureRef >> 32), loc.keys, uint32(loc.vertex)}
		if p.materialAttribute {
			material[1] = command.baseInstance
			command.baseInstance = uint32(len(p.materials))
		}
		p.commands = append(p.commands, command)
		p.materials = append(p.materials, material)
		p.runs[len(p.runs)-1].count++
	}
	// Many fragmented MultiDrawElementsIndirect runs interleaved with blend or
	// program state changes hit a steep GPU slow path on Windows AMD OpenGL
	// drivers; fall back to instanced draws when average batching per run is low.
	if len(p.commands) == 0 || len(p.runs) >= len(draws) || (len(p.runs) > 16 && len(p.commands) < len(p.runs)*8) {
		if p.reclaimDead {
			p.releaseGeometry()
			p.reclaimDead = false
		}
		return false
	}
	return true
}

func (p *effectIndirect) newRun(draw queuedEffect) {
	run := effectIndirectRun{first: len(p.commands), serial: -1, indexType: draw.mesh.indexType, src: draw.src, dst: draw.dst}
	if len(p.runs) < cap(p.runs) {
		i := len(p.runs)
		p.runs = p.runs[:i+1]
		run.textures = p.runs[i].textures[:0]
		p.runs[i] = run
	} else {
		p.runs = append(p.runs, run)
	}
}

func (p *effectIndirect) newSerialRun(index int) {
	run := effectIndirectRun{serial: index}
	if len(p.runs) < cap(p.runs) {
		i := len(p.runs)
		p.runs = p.runs[:i+1]
		run.textures = p.runs[i].textures[:0]
		p.runs[i] = run
	} else {
		p.runs = append(p.runs, run)
	}
}

func (p *effectIndirect) draw(s *Scene3D, pipe *effectPipeline, draws []queuedEffect) {
	if p.vao == 0 {
		gl.CreateVertexArrays(1, &p.vao)
	}
	if p.boundVertexCapacity != p.vertices.capacity || p.boundIndexCapacity != p.indices.capacity {
		bindMeshLayout(p.vao, p.vertices.id, p.indices.id)
		p.boundVertexCapacity, p.boundIndexCapacity = p.vertices.capacity, p.indices.capacity
	}
	p.commandData.upload(len(p.commands)*indexedDrawStride, unsafe.Pointer(&p.commands[0]))
	p.drawData.upload(len(p.materials)*int(unsafe.Sizeof(p.materials[0])), unsafe.Pointer(&p.materials[0]))
	if p.materialAttribute {
		bindDrawMaterial(p.vao, p.drawData.id, int32(unsafe.Sizeof(p.materials[0])))
	}
	var indirectBound, serialBound bool
	var boundKeys uint32
	for _, run := range p.runs {
		if run.serial >= 0 {
			if !serialBound {
				gl.UseProgram(pipe.program)
				gl.Uniform4fv(0, 1, &s.ClipPlane[0])
				gl.Uniform1i(2, 0)
				boundKeys = 0
				serialBound, indirectBound = true, false
			}
			boundKeys = pipe.drawSerialOne(s, draws[run.serial], boundKeys)
			continue
		}
		if !indirectBound {
			gl.UseProgram(p.program)
			gl.Uniform4fv(0, 1, &s.ClipPlane[0])
			gl.BindBufferBase(gl.SHADER_STORAGE_BUFFER, 5, p.keys.id)
			if !p.materialAttribute {
				gl.BindBufferBase(gl.SHADER_STORAGE_BUFFER, 7, p.drawData.id)
			}
			gl.BindVertexArray(p.vao)
			gl.BindBuffer(gl.DRAW_INDIRECT_BUFFER, p.commandData.id)
			indirectBound, serialBound = true, false
		}
		s.effectBlend(run.src, run.dst)
		if !p.materialAttribute {
			gl.Uniform1i(1, int32(run.first))
		}
		run.textures.bind()
		gl.MultiDrawElementsIndirectWithOffset(gl.TRIANGLES, run.indexType, uintptr(run.first*indexedDrawStride), int32(run.count), 0)
		drawCalls++
	}
	gl.BindBuffer(gl.DRAW_INDIRECT_BUFFER, 0)
	if p.reclaimDead {
		p.releaseGeometry()
		p.reclaimDead = false
	}
}

func (p *effectIndirect) releaseGeometry() {
	if p.vao != 0 {
		gl.DeleteVertexArrays(1, &p.vao)
		p.vao, p.boundVertexCapacity, p.boundIndexCapacity = 0, 0, 0
	}
	p.vertices.delete()
	p.indices.delete()
	p.keys.delete()
	p.geometry = nil
}

func (p *effectIndirect) delete() {
	p.releaseGeometry()
	p.commandData.delete()
	p.drawData.delete()
	if p.program != 0 {
		gl.DeleteProgram(p.program)
	}
}

// ReleasePackedEffects discards packed copies before the owning scene changes.
func (s *Scene3D) ReleasePackedEffects() {
	if s != nil {
		s.effectPipeline.indirect.releaseGeometry()
	}
}
