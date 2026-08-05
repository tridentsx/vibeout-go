# export-models — all 48 PRM models → glTF binary (.glb) at highest fidelity

Goal: one command exports every runtime WipEout 2097 `.PRM` model to `.glb` at
highest fidelity — geometry, face/vertex colors, embedded textures, and sprites.

    go run ./cmd/export-models --all            # all 48 runtime PRMs
    go run ./cmd/export-models COMMON/VECTO.PRM  # a single model

Reuses the existing, disc-validated `internal/psx` decoders (PRM/CMP/TIM) and
`internal/assets`. Adds a neutral mesh layer, a glTF writer, and a CLI.

## Confirmed model (verified against the extracted disc, `assets/WIPEOUT2`)

Texture pairing and UVs were probed on TERRY/WRECK/WIERD/TRAIN/SROID/RESCU and
TRACK01 SCENE/SKY:

- Textures live in the **same-basename CMP**: `FOO.PRM` → `FOO.CMP`. Untextured
  models (VECTO, VENOM, RAPIE, PHANT, JULIE, ASTER, …) have no CMP.
- `Polygon.Texture` is a **direct index into the CMP's ordered TIM members**
  (`maxIndex < pageCount` in every file).
- UVs are **pixel coordinates within the indexed page**; normalize
  `u/page.W, v/page.H` with **no V flip** (glTF UV origin is top-left, same as
  the decoded image; wipeout.js flips only because WebGL is bottom-left).
- Colors use the PS1 GTE **128 = 1.0** rule → `COLOR_0` byte
  `= min(round(c*255/128), 255)`. Emitted on textured primitives too (the GTE
  modulates the texture by this color).
- TIM decode already keys pure black (0x0000) to alpha 0 → transparent texels.
- Placement: every object has `Position == Origin`, vertices are model-space
  centered near 0, and `world = Position + vertex`. Emit each PRM object as its
  own glTF node with `Translation = axisConvert(Position)` and vertices
  `axisConvert(raw) = (x, -y, -z)`. That axis map has determinant +1, so
  triangle winding is preserved (no index reversal).
- Only polygon types 1–8 (+ sprite 11) occur across all 48 models; the parser
  already decodes UV/color for all of them, so nothing else needs RE.
- Sprites (type 10/11; present only in track `SCENE.PRM`, 0 in crafts):
  reference a page via `Texture`, use the whole page, size = `SpriteWidth/Height`
  in model units, anchored at `Vertices[SpriteIndex]`. Baked as static quads
  (glTF has no billboard); acceptable for a static export.

## Packages (deps point inward; no SDL below `render`; mirrors export-video/video)

- `internal/model` (new, SDL- and glTF-free): `FromPRM([]psx.Object, []Page)
  *Mesh`. Triangulates quads (fan 0,1,2/0,2,3), expands per-corner UV/color into
  per-vertex attributes with a corner-dedup map, applies the axis map and
  128=1.0 color, groups triangles into `Primitive`s by material (texture page
  index, or `Untextured`), and bakes sprite quads. Pure; table-tested.
- `internal/assets` `LoadModel(parts...) (*Model, error)` (new): decode the PRM;
  if any polygon is textured/sprite, decode the sibling `<base>.CMP` members to
  TIM pages (`[]*psx.Image`, nil for non-TIM members). Untextured models return
  no pages. Missing CMP for a textured model → warning, geometry still exports.
- `internal/glb` (new; only importer of `qmuntal/gltf`): `BuildDocument(name,
  *model.Mesh, []*psx.Image, Options) (*gltf.Document, error)` + `Save`. One
  image+sampler(nearest,clamp)+texture+unlit material per page (baseColorTexture,
  `AlphaMask` cutoff 0.5, DoubleSided), plus one untextured unlit material; one
  glTF mesh+node per PRM object; primitives carry POSITION, COLOR_0, and (when
  textured) TEXCOORD_0. Embedded PNG via `modeler.WriteImage`.
- `cmd/export-models/main.go` (new): the CLI. `go mod tidy` flips
  `qmuntal/gltf` from indirect to direct.

## Implementation order

1. [x] Parsing — reuse `internal/psx` (PRM/CMP/TIM), disc-validated.
2. [ ] `internal/model` + table tests (triangulation, corner de-dup, axis map,
   128=1.0 color, UV normalization, sprite quads).
3. [ ] `internal/assets.LoadModel` + test.
4. [ ] `internal/glb` + test (document has expected images/materials/primitives;
   `.glb` re-opens via `gltf.Open`).
5. [ ] `cmd/export-models` CLI: single + `--all` (walk disc, skip the 3 editor
   PRMs `COMMON/SKY.PRM`, `COMMON/TRACK.PRM`, `TRACK08/TRAK2.PRM`).
6. [ ] `go mod tidy`; `go build/vet/test ./...`.
7. [ ] Run `--all`, confirm 48 valid GLBs; re-open each and check geometry +
   materials + embedded textures; spot-check a textured model has images.

## Notes / limits

- Materials are unlit (`KHR_materials_unlit`) — matches the PS1's unlit,
  color-modulated, nearest-filtered look; normals are irrelevant and omitted.
- `DoubleSided` on by default (some 2097 faces are authored back-facing); a
  `--fix-winding` option can be added later if a specific model needs it.
- Sprites bake to static quads (no runtime billboard); only affects track
  `SCENE.PRM`, not crafts.
- Follow-up (not in this cut): refactor `render/model.go` to build geometry via
  `internal/model` so the runtime renderer and exporter share one conversion.
