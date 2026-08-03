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

## Runtime modules

Subsystem boundaries are now explicit; see `docs/architecture.md` for the
dependency rules. `game` owns state, `physics` owns simulation, `render` owns
SDL presentation resources, `assets` composes the low-level `psx` decoders,
and `audio/sfx` and `audio/music` have independent playback contracts. `main`
is only the composition root.

## Asset pipeline

`internal/psx/` — original PS1 asset format parsers. Binary-confirmed retail
layouts take precedence over the older reference implementation and are
validated against the extracted corpus:

- [x] `tim.go` — `.TIM` texture decoder (4bpp/8bpp paletted, 16bpp
      true-color), validated across all 53 files. The previously reported 11
      menu/title failures were stale; they are valid large 16bpp TIMs ranging
      from 320x240 to 640x256 and decode normally.
- [x] `cmp.go` — `.CMP` compressed bundle unpacker (custom bitfield-driven
      LZ77 variant).
- [x] `track.go`, `vew.go`, `chk.go`, `tex.go`, `ttf.go` — retail track
      vertices/faces/sections, section visibility lists, checkpoints, binary
      track texture assignments (plus text texture manifests), and 42-byte
      game-specific TTF records. Their endian handling follows the retail
      loaders: TRV/TRF/TRS/VEW/TTF are big-endian and explicitly swapped;
      CHK and TRACK.TEX are copied in native little-endian form.
- [x] `wad.go` — little-endian 25-byte WAD directory entries and payload
      extraction, validated across all 11 archives. Every corpus entry is
      uncompressed (`flags=0`, stored size equals unpacked size).
- [x] `menu.go` — `COMMON/MENU.DAT` line-art table. The retail loader copies
      4,212 little-endian 16-byte records and ignores 351 retained trailing
      records; both sets are parsed and preserved separately.

### Retained development artifacts (not retail asset loaders)

The remaining track-development extensions were searched across the main
executable, all five animation executables, every WAD directory/payload, every
decompressed CMP member, and ASCII/UTF-16/16-bit-swapped/32-bit-swapped filename
representations. None is requested by retail code:

- `.INF` — CRLF converter reports with source database/scene paths and the
  exact `track10`/scene conversion command line.
- `.MNU` — human-readable track build menus invoking `gettrk`, `getscn`, and
  `mkwad`.
- `.OUT`/`.BAK` — converter logs and backups; `.RST` — editor state files.
- `.VPO`/`.VRA` — text VRAM packing manifests naming source TIM paths and
  screen rectangles.
- `.SCN`, `.ROB`, and `RACELINE.BIN` — binary source/intermediate track data;
  their names are absent from the executables and WADs. Runtime products are
  the specialized TRV/TRF/TRS/VEW/CHK/TTF/TEX and SCENE/SKY PRM files instead.

These files belong to an optional future editor/converter importer, not the
retail loading compatibility target. They should remain preserved and clearly
classified rather than being accepted by unrelated runtime decoders.

- [x] `av.go` / `mdec.go` — fully decode all five retail intro `.AV` streams.
      The files are 2048-byte cooked sectors with seven standard STR/MDEC video
      sectors followed by one headerless 4-bit stereo XA ADPCM sector. The
      XTRO1 executable confirms the version-1 MPEG run/level VLC expansion and
      PSX MDEC DMA path; the Go decoder performs VLC expansion, dequantization,
      IDCT, 4:2:0 color conversion, and RGBA output. Corpus validation covers
      4,204 video frames and 3,157 audio sectors, including zero-filled unused
      video slots after several final frames. Sampled first/middle/final frames
      from every movie decode successfully, and independent FFmpeg comparison
      of a reference frame measured 51.1 dB PSNR (rounding-only differences).
- [x] `prm.go` — retail `.PRM` 3D model parser, including object normals and
      all 22 valid primitive record sizes from `LoadPrm`/`IntelPrim`. It
      decodes all 48 runtime PRMs. Three retained development files
      (`COMMON/SKY.PRM`, `COMMON/TRACK.PRM`, `TRACK08/TRAK2.PRM`) use an
      expanded editor/interchange layout and are intentionally classified
      separately: neither their exact names nor endian-swapped spellings are
      referenced by the executable, CMP payloads, or WAD directories. Every
      retail track WAD instead contains `track.trv`, `track.trf`, `track.trs`,
      `track.vew`, `scene.prm`, and `sky.prm`. `TRACK.INF` records the PC-side
      `track10` conversion command that produced those specialized runtime
      files from source scenes. Do not add guessed editor record sizes to the
      retail decoder; reverse engineer that source format separately if an
      editor/importer is wanted.
- [x] `cmd/inspect` — CLI for spot-checking parser output against real
      files during development.
- [x] `vag.go` — VAGp header and PS1 SPU ADPCM decoder, validated across all
      39 samples in `SAMPLES.WAD`. The binary-confirmed loader uses big-endian
      header size/rate fields and uploads data at offset 0x30. `three.vag`
      retains 448 bytes after its declared upload length; these are preserved
      but ignored exactly as the retail loader does.
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

## Game logic port (`internal/game/` state, `internal/physics/` simulation)

Started converting confirmed reverse-engineered logic into Go, per the
authority rule above:

- [x] `angle.go` — the original's 12-bit (4096-unit) circle convention as an
      `Angle` type, `Sin`/`Cos` via Go's `math` package rather than porting
      the original's lookup table (see file comment for why).
- [x] `ship.go` — `Ship` struct covering the confirmed subset of the
      original 240-byte ship struct (position, velocity, pitch/yaw/roll,
      flags, section ID, speed/max speed, track progress, rank counters).
      Deliberately partial — only fields with independently-confirmed
      semantics are included; each field cites its original struct offset
      for cross-referencing bn-psx's decompilation. `SectionID`'s offset
      was corrected in session 8 (was wrongly `[ship+0x98]`, actually
      `[ship+0xc8]/[+0xca]` — see `throttle.go`'s entry below for what
      `+0x98` really is).
- [x] `physics.go` — `UpdatePhysics`, porting the confirmed velocity-decay +
      position-integration core (`v -= v>>4`, `position += v>>5` in the
      original, ported as float32 multiplies). Deliberately does *not* yet
      cover steering input, banking/lean, or collision recovery — those
      parts of the original functions aren't decoded closely enough to port
      with confidence yet.
- [x] `throttle.go` — `UpdateThrottle`, porting the confirmed throttle/speed
      ramp from `maybe_IntegrateShipPhysicsFromPadInput`'s opening block
      (session 8): `Speed` ramps toward a throttle-derived target at a
      fixed rate, clamped to `MaxSpeed`. Verified via raw disassembly, not
      just decompiled pseudo-C, after finding the decompiler had flattened
      two different pad-state bit tests into one misleading branch.
- [ ] **Thrust-to-velocity model** — session 8 found the real formula in
      `maybe_IntegrateShipPhysics` (`0x80030784`): `thrust = Speed *
      forwardVector * boostMultiplier`, fed through a spring-like
      acceleration term into velocity. NOT yet ported — `[ship+0x90]`/
      `[+0x92]` (the spring denominator) and the boost multiplier's real
      trigger aren't confirmed, and porting them as guesses would violate
      the project's stated care about not porting unconfirmed constants.
      See `bn-psx/docs/wipeout2097_ship_physics_hunt.md` session 8.
- [x] `steering.go` — `UpdateSteeringDigital` + `IntegrateYawFromSteering`,
      porting the confirmed digital-input steering-rate ramp and its
      yaw-integration tail from `maybe_RunShipAutopilot`/
      `maybe_IntegrateShipPhysics` (session 8). This also reframes the
      earlier "steering input" blocker: `maybe_RunShipAutopilot` turns out
      to read `padInputState` directly and isn't AI-exclusive — see the
      hunt doc's session 8 addendum for why `maybe_IntegrateShipPhysicsFromPadInput`
      (zero static callers, session 7) is now suspected dead code rather
      than the real human-input path.
- [ ] Analog-stick steering path and bank/lean/pitch/roll — found (session
      8) but deliberately not ported: the analog clamp's exact bit
      manipulation isn't disassembly-verified, and bank/lean interacts
      with un-traced collision-avoidance code. See the hunt doc.
- [x] ~~Confirm whether the local player goes through this same steering
      path~~ — investigated and dead-ended cleanly: `maybe_RunShipAutopilot`
      itself has zero static callers (Binary Ninja's own xref database,
      a raw pointer-table byte scan, and a constant-propagation scan of
      `maybe_RaceMain` all found nothing), so the per-ship dispatch
      mechanism is unrecoverable from static analysis for *any* ship, not
      just the two leaf integrator functions from session 7. Since
      `UpdateThrottle`/`UpdateSteeringDigital` are driven purely by
      `padInputState` and per-ship stats (nothing AI-specific), reusing
      them for the local player is a reasonable, explicitly-flagged
      engineering assumption — not blocked guesswork, just not a
      confirmed fact. See the hunt doc's session 8 addendum.
- [ ] AI waypoint-following (`maybe_RunShipAutopilot`'s track-curvature-based
      synthetic input generation) — identified but not closely decoded.
- [ ] Collision/effect-callback-chain system (the `[ship+0xec]`/`[+0xe0]`
      convention) — identified as a real mechanism (timed effects like
      spin-out recovery) but not ported; needs its own design decision for
      how to represent in Go (probably a small state machine, not a literal
      function-pointer port).

### 🔴 OPEN ITEM: camera system is a placeholder, not reverse-engineered

`internal/render/camera.go`'s `NewChaseCamera` is a **standard, hand-picked
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
