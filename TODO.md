# wipeout-go — plan and TODO

Reimplementing WipEout 2097 in Go, aiming for the exact same game feel and
logic as the original, eventually in high resolution with custom
replacement assets. This file tracks the architecture decisions made so
far and the concrete work still ahead.

## Authority rule

`bn-psx`'s reverse-engineered analysis of the real compiled `SLES_003.27`
binary is the source of truth for ALL game logic — physics constants,
algorithms, race rules, AI behavior. `wipeout-rewrite`
(`/Users/tridentsx/src/ps1/wipeout-rewrite`, phoboslab's C reimplementation
of the *original* 1995 WipEout) is structural/conceptual reference only —
module boundaries, naming ideas, "what subsystems exist" — and must NEVER
override what the real binary shows. Its own README also notes no license
and possible leaked-source provenance, so no code gets copied from it
verbatim; only concepts, re-derived into fresh Go code from our own
reverse engineering.

## Stack: one dependency covers almost everything

**Go + `github.com/Zyko0/go-sdl3`** (purego-based, no CGo). Chosen over the
Kaiju Go engine, which turned out to be a monolithic engine-with-editor
(own `go.mod`, built by cloning its whole repo), not an importable
library — a poor fit for porting the PS1's own primitive-based rendering
model.

`go-sdl3` bundles everything needed as sub-packages, all confirmed working
directly (not assumed):

- **`sdl`** — window, GPU rendering, gamepad, core audio. Window/event
  loop smoke-tested and confirmed rendering on screen.
- **Gamepad input**: `sdl.Gamepad` fully wraps SDL3's `SDL_Gamepad` API —
  `Button`/`Axis`/`Type` (identifies PS4/PS5 specifically), `SetLED`,
  `NumTouchpads`, `SensorData` (gyro), `RumbleTriggers`, `SendEffect` (raw
  HID reports, for full DualSense adaptive-trigger control later).
  Bluetooth pairing happens at the OS level; SDL3's community-maintained
  mapping database already knows DualShock 4 / DualSense layouts. **No
  separate controller library needed.**
- **`mixer`** (SDL3_mixer binding) — confirmed via a live decoder-list
  check: bundles `WAV`, `STBVORBIS` (OGG), `DRFLAC` (FLAC), `DRMP3` (MP3),
  `AIFF`, `AU`, `VOC` natively. Covers both SFX and background music via
  the same `Track`/`Group`/`Mixer` abstraction (independent volume groups
  for music vs. SFX). **The planned CDDA-soundtrack-to-FLAC conversion
  will just work** — no extra decoding library needed.
- **`ttf`** (SDL3_ttf binding) — not yet exercised, but present for
  rendering the replacement TTF fonts once the HUD/text subsystem exists.

## Rendering approach

The PS1 has no 3D engine of its own: the GTE (COP2) transforms vertices to
screen space (affine, not perspective-correct — the source of the
original's characteristic texture warping on large polygons), and the GPU
is a pure 2D rasterizer with a small fixed primitive set (flat/gouraud/
textured triangle & quad, sprites, lines) and **no z-buffer** — depth
ordering was done entirely via the Ordering Table (a depth-bucketed linked
list, painter's algorithm).

Plan: port that same small primitive set directly onto SDL3's GPU API
(Vulkan/Metal/D3D12 under one interface), rather than adapting to a
general-purpose engine's mesh/material/scene-graph abstraction. Since
we're targeting high-res/modern rather than bit-exact PS1 behavior:

- Use a **real z-buffer** instead of porting the OT/painter's-algorithm
  sort — removes a whole class of sorting artifacts the original had to
  work around, and we're not hardware-constrained the way the PS1 was.
- Perspective-correct texture mapping instead of replicating GTE's affine
  warping is an easy option once we're not bound to GTE math — explicitly
  **not decided yet**, revisit once the base renderer works.
- Vertex transform math: plain float32 matrices are almost certainly fine;
  no need to replicate GTE's fixed-point quirks unless bit-exact behavior
  ever matters.

## Asset pipeline

`internal/psx/` — original PS1 asset format parsers, ported field-for-field
from phoboslab's `wipeout.js` (the working reference for these formats),
validated against all 174 `.TIM`/`.CMP`/`.PRM` files on the real disc:

- [x] `tim.go` — `.TIM` texture decoder (4bpp/8bpp paletted, 16bpp
      true-color). Note: a handful of `.TIM`-extension files on the disc
      (`LEGALPAL.TIM`, `MENUPIC.TIM`, `WIPTITLE.TIM`, others) aren't
      conventional single images — returns a clean error, but what these
      files actually are is still uninvestigated.
- [x] `cmp.go` — `.CMP` compressed bundle unpacker (custom bitfield-driven
      LZ77 variant).
- [x] `prm.go` — `.PRM` 3D model parser (objects/vertices/polygons).
      Note: polygon type `0x00` is an open question even upstream
      (phoboslab's own README: "possibly padding?") — `DecodePRM` returns
      partial results rather than failing the whole file over it.
- [x] `cmd/inspect` — CLI for spot-checking parser output against real
      files during development.
- [ ] **VAG decoder** — original SFX format (PS1 ADPCM, confirmed
      referenced as e.g. `shipship.vag` in the binary's debug strings).
      Needs its own small decoder, same shape of work as the parsers
      above; VAG's ADPCM scheme is simple and well-documented.
- [ ] Texture/font **replacement/override layer** — deliberately deferred.
      The two HD texture packs already downloaded
      (`~/Downloads/WipEout-2097-HD-Texture-Pack.zip`, `WipEout 2097-XL -
      DuckStation Pack`) are keyed by **emulator-internal VRAM-upload
      content hashes** (DuckStation's and Beetle PSX HW's own runtime
      texture-capture hashes respectively), not by any asset identity a
      from-scratch parser produces — the filenames carry no meaning we can
      use directly. Plan: once our own TIM/PRM parsing gives us a stable
      texture-identity scheme (which file/track/offset), do a one-time
      matching pass (likely semi-automatable via image comparison against
      the low-res originals) to build our own lookup table. Do this
      *after* the base renderer works with original assets, not before.
- [ ] TTF font wiring — not blocking; do once the HUD/text-rendering
      subsystem exists.

## Reverse-engineering status (ship physics/game loop)

Full details and exact next steps live in
`bn-psx/docs/wipeout2097_ship_physics_hunt.md` — not duplicated here since
that doc is the actively-maintained source of truth for this effort.
Current state: the real per-ship physics integrator is identified with
strong confidence — `maybe_IntegrateShipPhysics` (AI ships, fed synthetic
pad-shaped input by `maybe_RunShipAutopilot`) and
`maybe_IntegrateShipPhysicsFromPadInput` (real pad input, structurally
parallel, no AI-exclusion check) both confirmed writing position, velocity,
and all three orientation axes (pitch/yaw/roll) every frame. The exact
runtime dispatch connecting real/synthetic input to these functions was
*not* found despite an exhaustive, verified search of every control-transfer
instruction in the executable (confirmed not an overlay either — checked
against the actual disc contents) — this remains the one open question in
an otherwise-resolved physics picture.

## Game logic port (`internal/game/`)

Started converting confirmed reverse-engineered logic into Go, per the
authority rule above:

- [x] `angle.go` — the original's 12-bit (4096-unit) circle convention as an
      `Angle` type, `Sin`/`Cos` via Go's `math` package rather than porting
      the original's lookup table (see file comment for why).
- [x] `ship.go` — `Ship` struct covering the confirmed subset of the
      original 240-byte ship struct (position, velocity, pitch/yaw/roll,
      flags, section ID, track progress, rank counters). Deliberately
      partial — only fields with independently-confirmed semantics are
      included; each field cites its original struct offset for
      cross-referencing bn-psx's decompilation.
- [x] `physics.go` — `UpdatePhysics`, porting the confirmed velocity-decay +
      position-integration core (`v -= v>>4`, `position += v>>5` in the
      original, ported as float32 multiplies). Deliberately does *not* yet
      cover steering input, banking/lean, or collision recovery — those
      parts of the original functions aren't decoded closely enough to port
      with confidence yet.
- [ ] Steering input application (blocked on the dispatch-mechanism gap
      above — need to pin down how pad input actually reaches the
      integrator before porting how it's used).
- [ ] AI waypoint-following (`maybe_RunShipAutopilot`'s track-curvature-based
      synthetic input generation) — identified but not closely decoded.
- [ ] Collision/effect-callback-chain system (the `[ship+0xec]`/`[+0xe0]`
      convention) — identified as a real mechanism (timed effects like
      spin-out recovery) but not ported; needs its own design decision for
      how to represent in Go (probably a small state machine, not a literal
      function-pointer port).

### 🔴 OPEN ITEM: camera system is a placeholder, not reverse-engineered

`internal/game/camera.go`'s `NewChaseCamera` is a **standard, hand-picked
third-person chase camera** (behind + above the ship, following its
heading) — explicitly **not** derived from the real binary, unlike
everything else in `internal/game/`. This breaks from the authority rule
above and needs closing out.

What's actually confirmed from bn-psx so far: `maybe_TransformAndSubmitPolygons`
(SLES_003.27 `0x80012ed4`) loads a precomputed camera-relative transform
matrix from `[drawObject+0x30]` straight into the GTE before `gte_rtps()`
— the standard PS1 pattern of combining camera and object world transforms
once per object per frame, not per-vertex. The draw-list table this reads
from (`0x800f6f24`) was located, but tracing where each entry's `+0x30`
matrix actually gets *written* (and from there, the real camera
position/orientation formula relative to the ship) was not done — a
genuinely open RE task, not just unstarted busywork.

**To close this item**: reverse-engineer that write site in bn-psx, update
`bn-psx/docs/wipeout2097_ship_physics_hunt.md` with the findings, then
replace `NewChaseCamera`'s body (not just its constants) with the real
formula, updating the "PLACEHOLDER CAMERA" comment in `camera.go`
accordingly.

## Open decisions (not yet made, don't assume)

- Perspective-correct vs. affine texture mapping (see Rendering approach).
- Exact texture-identity scheme for the replacement-asset matching pass.
- Whether to replicate the PS1's fixed-point GTE math exactly anywhere, or
  use float32 throughout (current lean: float32 throughout, unless a
  specific case demands otherwise).
