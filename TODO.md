# wipeout-go — plan and TODO

The executable's four difficulty-specific sixteen-slot ship-stat banks are
now represented by `game.RaceShipSpec`. It supplies the exact inertia,
maximum speed, drag, turn acceleration, and turn rate loaded during
`InitializeRaceShipsAndStartingGrid`; turn values preserve the original
integer `*60/50` normalization. The TRACK01 preview uses difficulty 0, slot 0
and now drives the confirmed input/grounded-physics/orientation/camera chain
from an SDL gamepad.

The temporary TRACK01 section viewer verified that decoded track geometry,
section connectivity, and perspective projection produce a recognizable track
outline. It also exposed and fixed a camera-basis sign error: the old Down
vector was not orthogonal to Forward. The executable now advances to the next
integration milestone: authentic starting-grid placement, the binary-backed
25 Hz input/physics chain under PAL's 50 Hz field presentation, the persistent
race camera, and a decoded VECTO.PRM
player craft. The section camera remains a diagnostic helper; its exact visual
centering is not gameplay authority and is intentionally no longer blocking.

`StepShipTrackPhysics` now performs the executable's flag-0x10 dispatch after
the nearest-section search. Ordinary sections preserve that state; jump
sections may set it by projecting the ship onto the paired driving-face plane
and testing polygon containment. It is not selected by a guessed
center-distance threshold. Near-track ships use the grounded surface-contact
path. Far-from-track ships use the spring/gravity path and conditionally call
the wall resolver while `Velocity.Y < currentSection.Y`. Thrust, speed magnitude, and the forward
redirect target are captured before that wall call, preserving the original
mutation order. The asymmetric current-to-next centerline
projection test and its 32001-unit recovery transition are now ported: it
zeros Speed, sets the ship timer to 500, and ORs flags 0x401. Installing and
executing the original recovery callback plus its global 1000-frame state are
still presentation/race-flow work.

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
- [x] `cmd/export-video` / `internal/video` — lossless single-container cutscene
      export for restoration tools: FFV1 video plus 18.9 kHz stereo PCM audio
      in Matroska. The stripped XA cadence confirms 225/16 fps (three audio
      sectors for every four frames), keeping exported audio/video durations
      synchronized without resampling.
- [ ] Video playback module — implement the `internal/video.Player` contract
      with incremental MDEC decoding, SDL audio queuing, audio-clock sync,
      cancellation/skip input, and optional preference for modern replacement
      MKV files. Do not decode an entire cutscene to RGBA in memory at runtime.
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

### ✅ Race camera and starting-grid placement

`internal/render/camera.go` now holds the persistent external/cockpit race
camera port. The renderer also uses the executable's confirmed GTE projection
distance (`H=1000` at `InitGTEProjectionState`, `0x8008008c`) rather than a
guessed field of view.

Confirmed camera entry points and behavior:

- `UpdateRaceStartCameraArc` (`0x800209f4`) walks backward through track
  sections, eases from a grid-facing point toward the player, continuously
  looks at the ship, and hands off at `ship+0xe0 == 0x64`. Race setup
  initializes that per-ship timer to `0xa6`; countdown tones occur at 125, 83, and
  0, so the camera handoff at 100 falls between the first two active tones.
- `UpdateChaseCameraFollow` (`0x80020608`) starts at
  `shipPosition - shipForwardQ12/4` with another `-200` on Y, projects that
  anchor onto the local track-centerline segment, and applies the original
  signed 16-bit spring, damping, and clearance-bias state. It follows ship
  pitch/yaw but deliberately forces camera roll to zero.
- `UpdateCockpitCameraView` (`0x800208fc`) places the camera at a rigid
  128-unit offset along the ship's rotated local-up vector and inherits
  ship pitch, yaw, and roll exactly.
- The race-start callback body is the split sibling at `0x800209f8`
  (Binary Ninja keeps the public callback entry at `0x800209f4`). It walks
  eight linked track nodes backward, builds a timer-driven quadratic easing
  term from `0xc8 - ship+0xe0`, interpolates the camera anchor from the
  backward track point toward the ship, then projects that anchor onto the
  nearby section and continuously aims at the ship. At `ship+0xe0 == 0x64`
  it replaces the callback with chase or cockpit view according to the
  two-player/view bit at `0x2a`, and updates ship flag `0x4` accordingly.
  The callback is invoked indirectly through `data_8009563c` by the race
  main loop; this explains why no direct xref to the camera routine exists.
- `ToggleShipCameraView` (`0x800663cc`) is bound to pressed-button bit
  `0x1000` and swaps the callback between the chase and cockpit routines;
  ship flag `0x4` records the selected view.

These identities and formulas come from aligned MIPS assembly and LLIL,
including restoration of their legitimate overlapping/tail-shared entry
points after Binary Ninja auto-analysis dropped them.

The normal racing external/cockpit camera and Change View state are ported.
The race-start sweep in `RaceCamera.updateRaceStart` is now live-verified
(bn-psx session 25, 2026-08-05): single-stepping a real DuckStation session
frame-by-frame through the intro sweep and comparing against the live camera
node's translation matched this formula to within +/-1 world unit on 9 of 10
sampled ticks across the full timer range -- bit-exact modulo PS1-vs-host
truncation direction. `main.go` enables it unconditionally via
`BeginRaceStart()`. Physics remains independent and the retail callback
handoff (`ship+0xe0==0x64`) was also confirmed live: the intro breakpoint's
hit count stops climbing at exactly 66 per sweep and the chase callback
fires for the first time on the very next tick.

`internal/game/grid.go` ports the position/orientation portion of
`InitializeRaceShipsAndStartingGrid` (`0x80022bbc`): every grid slot advances
two linked sections, alternates between the two grid faces, places the ship
at the midpoint of face vertices 0 and 2 plus `normal*75/1024`, assigns the
section, and sets yaw from the current-to-next section vector. The
executable's TRACK01 configuration selects start section 0.

### 🟡 OPEN ITEM: track contact and hover behavior

- Per-frame section progression is now ported as `UpdateShipTrackSection`.
  `UpdateShipTrackSectionNearest` (`0x80025674`) does not wait for a face
  boundary: it searches a seven-section main-route window beginning three
  `Previous` links behind the current node, plus six nodes on an encountered
  junction route. Its squared-distance metric uses full X/Z error and Y/4,
  retains the first candidate on ties, writes the old/new indices to
  ship+0xca/+0xc8, and toggles ship flag 0x10 at distance 3701. The grounded
  frame step now performs this search before track-side and face selection.

- `ProjectPointOntoLineThroughPoints` (`0x80031e8c`) was previously
  mislabeled as a ship surface routine. It is a shared geometric projection
  used by the camera, weapon orbit, and physics.
- `IntegrateShipPhysicsAndTrackContact` (`0x80030788`) measures a ship probe
  against the selected driving face with `PlaneDistance`. Below 600 units it
  adds `5 + (arg2-distance)` to `ship+0x78` (confirmed `PitchRate`); otherwise
  it subtracts 50, then damps the result by one quarter. This is a contact/
  pitch-alignment response, not enough evidence by itself to call it the
  complete hover suspension.
- The same routine projects against the current/next section centerline for
  its 32001-unit out-of-bounds test and separately compares section-center Y
  against ship Y with thresholds 705 and 80. These paths must be decoded as
  a whole before wiring track contact into live movement.
- The paired driving-face selector is now confirmed and ported in
  `internal/physics/track_contact.go`. Per ship,
  `UpdateShipOrientationVectorsAndTrackSide` computes
  `dot(sectionCenter-shipPosition, face.vertex0-face.vertex1)`; a positive
  result sets ship flag `0x20`. That flag selects the first driving face when
  set and the immediately following face when clear.
- The contact-force numerator is ported independently of the still-open
  collision dispatch: per Q12 face-normal component it computes
  `normal*16384/max(distance,75) - normal*64`; Y additionally receives
  `+30000 + (currentSectionY-shipY)*64`. The section Y source is the current
  section pointer at `ship+4`, not the linked next section. TRACK01's actual first grid face has
  `NormalY=-4096`, and authentic grid slot 0 begins at PlaneDistance `-300`,
  so the 75-unit floor is active at race start. Therefore `256` is only the
  algebraic zero of the normal term on the opposite sign side, not evidence
  for a 256-unit hover equilibrium.
- A second PlaneDistance sample at `position + ForwardQ12/32` (128 units
  ahead) feeds pitch alignment: below 600 it adds
  `5 + centerDistance-forwardDistance` to `PitchRate`, otherwise subtracts
  50, then damps PitchRate by one quarter.
- `StepGroundedShipTrackPhysics` now composes the confirmed ordinary
  `ship.Flags&0x10 == 0` ordering: orientation/track-side refresh, pre-contact
  thrust and speed magnitude, wall response, clamped surface impulse,
  surface-force plus thrust acceleration, shared drag/position integration,
  then the forward-probe pitch response. It is covered against TRACK01's
  authentic grid slot 0. The section-height correction is also ported:
  at a gap of at least 705, inertia class 110 moves Y by 80; other classes
  halve negative Y velocity and move Y by 16. The ship+0x9e pitch bypass is
  retail-dormant: exhaustive aligned-store scanning found only its zeroing
  initializer at `0x8002317c` and no nonzero writer.
- `ApplyTrackSurfaceContactImpulse` (`0x800337c0`) is no longer confused
  with wall collision. For distance `>=31` it does nothing; for
  `0<distance<31` it adds `normalQ12*60/50`; for non-positive distance it
  first damps velocity by `1/8`, then adds
  `normalQ12*(60/50)*(1-distance/16)`. The physical core is ported and
  tested; its particle/SFX call remains presentation work.
- `ResolveShipWallSensorCollisions` (`0x80033c1c`) ignores the extra `a2`
  value left by its physics caller; pad input does not select collision
  behavior. It constructs a 512-unit forward probe plus paired side probes.
  Only `PlaneDistance<=0` candidates dispatch a response. When
  `section.CollisionFlags&0x180000 != 0`, `PointInsideFace` must succeed and
  the graze response is used. Otherwise the first accepted sensor uses the
  graze response and subsequent accepted sensors in that sweep use the full
  response. This selection is ported. The two direction-dependent edge tests
  are also exact: each tests the pair
  `position +/- orientationRow2Q12/16 +/- ForwardQ12/16`, i.e. corners 256
  units to either side of an edge 256 units ahead of or behind the ship.
  `WallSensorEdge` ports that geometry without guessing which section-travel
  branch semantically means front/rear. The face-range traversal is now also
  ported as `SectionWallSweep`: the sign of
  `dot(sectionCenter-shipPosition, drivingFace.vertex0-vertex1)` selects the
  non-track prefix or suffix surrounding the section's contiguous run of
  Track faces. A corpus check of every extracted matching TRS/TRF pair found
  no section with a non-contiguous Track-face run. Resolving the junction-
  neighbor fallback tree, then wiring the responses into live movement, is
  the remaining wall-collision task. `SampleSectionWallSensors` now composes
  that exact face run with the executable's 512-unit nose probe and paired
  256-unit edge corners, returning all three signed plane distances without
  prematurely applying a response. The prefix-side nose probe's complete
  junction containment tree is now ported as
  `SelectPrefixNoseResponseFace`. It covers current JunctionStart/End,
  `previous.NextJunction`, `next.NextJunction`, and the executable's ordered
  candidate/previous/next/junction first-face fallbacks. Neighbor tests reuse
  the original candidate's plane distance, matching the call sites. The
  mirrored suffix-side tree is now ported too. It correctly uses neighboring
  sections' fixed `FirstFace+3` right-wall slots and retains its asymmetric
  response-face substitutions. `SelectWallNoseContacts` composes both sides,
  the face sweep, and nose PlaneDistance into accepted hard-response records.
  The hard response's gameplay transition is now ported as
  `HardWallCollisionResponse`: negative-distance velocity braking, the common
  anisotropic normal impulse, position integration at velocity/64, and the
  signed 16-bit steering kick of `SpeedMagnitude/4+400`. Final live response
  The second orientation row is now confirmed as local Right and exposed on
  the runtime ship. Its exact yaw/pitch/roll formula from
  `UpdateShipOrientationVectorsAndTrackSide` is evaluated alongside Forward,
  so the wall-corner probes no longer need provisional orientation input.
  The end-of-sweep correction gate at `0x80094ae8` starts at zero and an
  exhaustive aligned-instruction scan found no nonzero writer anywhere in
  SLES_003.27; its only static writer clears it at `0x80042428`. The correction
  is therefore dormant in this retail executable unless an external overlay
  changes it. `ResolveShipWallSensorCollisions` now performs the authentic
  sequential gameplay transition: fixed nose and immediate hard response,
  recomputed negative-Right corner, then a newly recomputed positive-Right
  corner. It preserves junction face substitution, special-section
  containment, the routine-local collision flag, both steering-kick scales,
  and the fact that ordinary grazes do not set that flag. The former
  single-point helper is retained under the explicit
  `ResolveShipWallCollisionApproximate` name and is not the gameplay path.
  Remaining work is choosing the correct orchestration point alongside the
  still-partial track-contact integrator, then validating against real track
  sections in motion.

## Open decisions (not yet made, don't assume)

- Perspective-correct vs. affine texture mapping (see Rendering approach).
- Exact texture-identity scheme for the replacement-asset matching pass.
- Whether to replicate the PS1's fixed-point GTE math exactly anywhere, or
  use float32 throughout (current lean: float32 throughout, unless a
  specific case demands otherwise).
