# Whole-track pack format (`.trackpack`)

Status: v1, implemented. `cmd/encode-track` writes it and `internal/trackpack`
reads it (`Load`/`Decode` plus surface geometry helpers) for the renderer. The
format is self-describing and does not depend on the original disc at load time.

Surface tiles are the game's highest-resolution original road textures: the
composed 128×128 "near" tile per logical tile index (a 4×4 grid of the 32×32
`LIBRARY.CMP` sub-tiles). That is the encoder default; upscaling means replacing
those PNGs with larger images (UVs are normalized).

A pack bundles a complete WipEout 2097 track — **sky + surrounding scenery +
the driving surface with its tiles and gameplay triggers** — as modern,
upres-ready assets, while keeping the surface's tile-index and per-face
flag/trigger logic intact (never baked away).

## Design principles

- **Logic vs pixels.** A track splits into resolution-independent *logic*
  (geometry, tile *indices*, face *flags*/triggers, section graph, checkpoints)
  and resolution-dependent *pixels* (textures). Only pixels change to go hi-res.
- **The driving surface stays logical, not baked.** Its faces keep their tile
  index + flags; the renderer builds the mesh and applies the tile textures by
  index at load, exactly like the original. This preserves boost/weapon/
  start-grid/flip triggers.
- **Scenery and sky are pure visuals** (no per-face gameplay), so they are baked
  meshes (glTF), which is efficient and viewable.
- **Textures are upres-able.** Surface tiles are external PNGs keyed by the
  logical tile index (swap the file → hi-res, no re-encode). Scenery/sky textures
  are embedded in their `.glb`; to upres them, re-run the encoder with a hi-res
  texture set (the encoder "takes the current format plus new textures").
- **One coordinate space.** All geometry is emitted in glTF space
  (`(x, -y, -z)`, right-handed, Y-up), so the surface, scenery, and sky align in
  one world when a renderer loads them together.

## Pack layout

A directory (zip-able), one per track:

```
TRACK01.trackpack/
  track.json                 # metadata + surface logic + layer refs + texture manifest
  scenery.glb                # SCENE.PRM baked mesh (embedded textures)
  sky.glb                    # SKY.PRM baked mesh (embedded textures)
  surface/tiles/000.png      # driving-surface tiles, one per LOGICAL tile index
  surface/tiles/001.png      #   (TrackFace.Tile / TRACK.TEX index) — upres-swappable
  ...
```

## `track.json` (v1)

```jsonc
{
  "formatVersion": 1,
  "name": "TRACK01",
  "axes": "gltf",                 // (x,-y,-z), Y-up, right-handed
  "surface": {
    "tileCount": 12,
    "tileDir": "surface/tiles",
    "vertices": [[x,y,z], ...],   // int world units, glTF space
    "faces": [
      {
        "v": [i0,i1,i2,i3],       // indices into vertices (quad; tri repeats i3=i2)
        "normal": [x,y,z],        // float, unit
        "tile": 7,                 // index into surface/tiles/
        "color": [r,g,b],         // 0..255, PS1 128==1.0
        "flip": false,             // flip texture horizontally (flag bit 2)
        "flags": {                 // decoded triggers + raw byte
          "raw": 33, "track": true, "weaponPad": false, "flip": false,
          "weaponPad2": false, "special": false, "boost": true
        }
      }
    ],
    "sections": [
      { "prev": 1, "next": 2, "nextJunction": -1,
        "center": [x,y,z], "firstFace": 0, "numFaces": 12,
        "flags": { "raw": 0, "jump": false, "junction": false,
                   "junctionStart": false, "junctionEnd": false } }
    ],
    "checkpoints": [ { "file": "CPOINT0.CHK", "sections": [12,-1,-1,-1,-1,-1] } ]
  },
  "layers": {
    "scenery": { "file": "scenery.glb" },
    "sky":     { "file": "sky.glb" }
  },
  "textures": {
    "surface": [ { "tile": 0, "file": "surface/tiles/000.png",
                   "width": 128, "height": 128, "near": [/*16 sub-tile indices*/] } ],
    "scenery": { "source": "SCENE.CMP", "embeddedIn": "scenery.glb" },
    "sky":     { "source": "SKY.CMP",   "embeddedIn": "sky.glb" }
  }
}
```

Notes:
- Surface **UVs are not stored**: track faces use fixed corner UVs
  (`(1,1),(0,1),(0,0),(1,0)`, flipped in X when `flip`), so they are
  resolution-independent and derived at load.
- Surface tiles are the composed 128×128 "near" tiles keyed by logical index; a
  modern renderer relies on GPU mipmaps instead of the original near/med/far
  LOD, so the med/far tables are not carried.
- `visibility` (TRACK.VEW section-visibility lists) is intentionally omitted;
  a modern renderer uses frustum culling.
- Face flag bits (raw byte preserved; names per `internal/psx` — the WO2097
  meanings of bits 1/3/4 are still being confirmed against bn-psx):
  `1 track`, `2 weaponPad`, `4 flip`, `8 weaponPad2`, `16 special`, `32 boost`.
  Section flag bits: `1 jump`, `8 junctionEnd`, `16 junctionStart`, `32 junction`.

## Encoder: `cmd/encode-track`

`go run ./cmd/encode-track --all` or `... TRACK01`. Inputs: the extracted disc.

Steps:
1. `assets.LoadTrackSurface(name)` → vertices (TRV), faces (TRF with `TRACK.TEX`
   overrides applied), sections (TRS), checkpoints (`CPOINT*.CHK`), and composed
   tiles (`assets.LoadTrackTiles`, LIBRARY.TTF + LIBRARY.CMP).
2. Convert vertices/normals to glTF space; decode face and section flag bytes to
   named booleans (keeping `raw`); write `track.json`.
3. Write each composed tile to `surface/tiles/NNN.png` (or a supplied hi-res
   tile keyed by index, if given).
4. `assets.LoadModel("<track>/SCENE.PRM")` and `SKY.PRM` → `model.FromPRM` →
   `glb.BuildDocument`/`Save` → `scenery.glb`, `sky.glb` (reusing the existing
   model/glb pipeline; textures embedded).

Deterministic, no SDL, mirrors the existing exporters.

## Upres workflow

- Surface: replace `surface/tiles/NNN.png` with a hi-res image (any size; UVs are
  normalized). Nothing else changes.
- Scenery/sky: re-run `encode-track` with a hi-res texture set (future
  `--textures <dir>` keyed by the identities in `cmd/export-textures`'s manifest)
  so the baked `.glb` embeds hi-res pixels.

## Reading it (`internal/trackpack`)

`trackpack.Load(dir)` / `Decode(r)` parse `track.json` into typed structs. A
renderer builds the surface mesh from `Surface.Faces` via `Face.Triangles()`
(retail winding + fixed corner UVs) indexing `Surface.Vertices` (glTF space),
textures each face by `Face.Tile` through `Pack.LoadTile(i)`, loads
`scenery.glb`/`sky.glb` via `Pack.SceneryPath()`/`SkyPath()`, and keeps
`flags`/`sections`/`checkpoints` for gameplay. The package depends only on the
standard library; loading the `.glb` layers is left to the renderer's glTF
loader. Nothing here depends on the original disc.

## Decisions

- **Section visibility omitted.** `TRACK.VEW` per-section visibility lists are
  not carried; the renderer uses frustum culling. (Already reflected above.)
- **Scenery/sky textures embedded.** They live inside `scenery.glb` / `sky.glb`.
  Upres of scenery/sky is done by re-running the encoder with a hi-res texture
  set (the encoder "takes the current format plus new textures"). Only the
  driving-surface tiles are external files, and those stay file-swappable.

## Open choices (deferred)

- Binary `track.json` variant if load time ever matters (v1 is JSON for
  readability and modding).
