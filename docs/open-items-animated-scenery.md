# Open items: animated scenery, start sequence, race-start props

State at the end of the session that added animated scenery. Everything below is
either unimplemented or implemented with a known compromise; the finished parts
are noted only where they bound the open work.

## Done, for context

- **Fans** spin: one shared angle, `+100` per 25 Hz tick, applied as roll. Matches
  retail (`maybe_RotateFanObjects`, `0x8004903c`).
- **Billboards and smoke** cycle texture frames from `TEXTURES/SMOKE.CMP` (25
  frames) and `TEXTURES/<set>RED.CMP` (2 frames). Rates confirmed correct by eye.
- **Grid placement**: walked backwards from the start/finish line, `2k+1` sections
  per slot, with the staggered left/right face selection working.
- **Chase camera** vertical clearance now uses retail's separate accumulator
  (`+|err|/128`, `-1/16` decay) rather than WipEout 1's folded term.

## 1. Maintenance craft at race start (not started)

During the intro camera sweep a maintenance/rescue craft lowers the player's ship
onto a pad, is drawn at a different point on the track from the player, and then
flies away while the countdown runs.

Leads:

- The model is almost certainly `COMMON/RESCU.PRM` — one object, 51 polygons, 41
  vertices, with its own `RESCU.CMP` textures. Note the object inside it is named
  `"grid2"`, not `"rescu"`, so a name-prefix search for "rescu" will not find it.
- The timing window is the race-start camera: `RaceCamera.RaceStartTimer` runs
  `0xa6 → 0x64` and the port already models that handoff. The craft's departure
  overlaps the countdown, which is a separate timer at `raceState+0xe0`
  (`maybe_UpdateRaceStartCountdown`, `0x800251d0`, firing SFX at counts `0x29`,
  `0x53`, `0x7d`).
- `UpdateRaceStartCameraArc` (`0x800209f4`) is the intro camera callback and is a
  good place to look for what else it drives; it is one of the three camera
  callbacks that were only reachable by function pointer.

Unknown: where the craft's path comes from, and whether its position is scripted
or derived from the grid slot.

## 2. Start light gantry (blocked on loading `COMMON`)

The model is confirmed: `COMMON/LIGHT.PRM`, one object `"light1"`, 63 polygons of
which **29 are type 4** — the exact primitive type the tint loop filters on. A
byte-identical copy is inside `STARTWAD.WAD`.

Retail draws **three** instances of that one model, positioned and aimed per frame
by `maybe_UpdateStartLightGantry` (`0x80049ef4`) from a three-pointer placement
array (`maybe_StartLightPlacements`, `0x800f6f18`), with a `-0xbb8` Y offset and
`ratan2` aiming.

Colour is **per-primitive RGB written into the object's own primitives**, not a
palette or texture swap. Phases, from the byte stores:

| site | write | phase |
|---|---|---|
| `0x8004a050` | R=`0xff`, G=0 | red |
| `0x8004a06c` | R=`0xff`, G=`0xff` | yellow |
| `0x8004a078` | R=0, G=`0xff`, B=0 | green |
| `0x8004a0fc` | R=0, G=`v0++`, B=0 | green ramp (the chase) |
| `maybe_ResetStartLightColors` (`0x80049e5c`) | `0x80,0x80,0x80` | unlit |

Only the first 8 primitives of each object are considered, and only those with
type tag 4.

Note: an earlier analysis (`rendering-reverse-engineering.md`) ruled `LIGHT.PRM`
out as "a generic gray lamppost, no color in its palette at all". That reasoning
was inverted — a colourless grey model is exactly what runtime RGB tinting needs.

Blocker: the port loads assets per track and never opens `COMMON/` or
`STARTWAD.WAD`. Race init in retail does
`OpenWADFile("c:\wipeout2\common\startwad.wad")` and binds `"light"` and
`"sroid"` out of it.

## 3. Start-line sign, checkpoint and finish markers (blocked on the same)

Not investigated. `COMMON/SROID.PRM` is the obvious candidate for the `"sroid"`
binding that race init resolves alongside `"light"`. Checkpoint *sections* are now
decodable — `TrackFaceCheckpoint` (`0x80`) is named and TRACK01's are sections 5
and 139 — but whatever geometry marks them is not.

## 4. Smoke plumes appear where retail does not show them

Observed: some plumes are drawn in places normal gameplay never shows, and are
missing from places it does. Suggests per-object gating, plausibly on race state.

`maybe_AnimateSmokeTextureFrames` (`0x800481b4`) sets `*(prims + 2) |= 4` before
stamping frames — an unidentified flag bit that may be the enable, or may be the
GPU semi-transparency bit. Worth decoding before adding gating logic, since it
also bears on item 5.

## 5. Smoke may need additive/semi-transparent blending

Currently punch-through: the shader discards `alpha < 0.5`, reproducing the PS1
colour-key cutout. PS1 smoke commonly used semi-transparent primitives instead. If
the plumes read as hard-edged rather than soft, that `|= 4` above is the first
thing to check.

## 6. Trackside cameras are bound but not animated

The six `"camera"` objects bind and draw statically. Retail aims them at the player
and bobs them on a sine (`maybe_AimAndBobCameraObjects`, `0x8004a4b0`):

```c
yaw   = ratan2(dx, dz)
pitch = ratan2(dy, SquareRoot0(dx*dx + dz*dz))
if (target->0xc & 8) { yaw = 0x1000 - yaw; pitch = 0x1000 - pitch }
else                 { yaw = -yaw;         pitch = -pitch }
baseY[i] = node->y                       // cached on the first pass
node->y  = baseY[i] + (GetSin(phase) >> 6)
phase += 0xc8                            // per tick
```

Needs per-object animation state, which `AnimatedScenery` does not currently hold
(its fields are scalars shared across each set).

## 7. Only track ID 1 has scenery bindings

`TrackSceneryBindings` covers Talon's Reach only. All eight dispatch cases exist in
`maybe_RaceMain` (`0x8003f7dc`–`0x8004016c`) and the resource names are known
(`AGunit`, `screen`, `dish`, `grid1`, `torch`, `train`, `stewy`, `pylon`,
`zeppelin`), but the per-track blocks have not been transcribed. Billboard texture
sets are mapped for IDs 1, 6, 7 and 8 only.

## 8. Billboard panel UVs are derived, not read

Retail stamps each panel's UVs from the frame descriptor, offsetting by the
descriptor's width (`+0x12`) and height (`+0x14`) fields. The port instead derives
each panel's sub-rectangle from where its vertices sit in the object's plane. That
works — all four TRACK01 billboards map correctly — but it is a reconstruction, not
the retail data path, and may not hold for object types not yet seen.

## Cautions worth carrying

- **If an object is animated at runtime, distrust its authored UVs entirely.**
  Smoke quads reference an 8×8 placeholder texture and billboards a 4×4, because
  retail overwrites UVs and texture page every frame. Sampling with those UVs drew
  smoke as a solid block and billboards as a 2×2 tile of warning stripes.
- **Object names are matched by prefix, not equality.** `maybe_LoadIndexedResource`
  uses `strncmp(name, wanted, strlen(wanted))`. TRACK01 has no object called
  `smokes` — it has `smokes1`, `smokes3`, `smokes4`.
- **The per-name integers in the dispatch chain are `strncmp` lengths, not
  counts.** `fan` passes 3, `redb` 4, `camera` 6 — each equal to its own `strlen`.
- **Menu order is not the internal track ID.** TRACK01 is menu index 0 but internal
  ID 1, and the per-track tables are keyed by the internal ID. Indexing by menu
  order reads entry 0, which is usually zero.

---

# Update: findings from reading the executable

Full detail in `bn-psx/docs/wipeout2097_race_start_props.md`. Summary of what
changed above.

**Items 2 and 3 share one blocker, now precisely located.** All three missing props
are owned by `maybe_InitGlobalRaceResources` (`0x80041e9c`), a one-time init from
`main` that opens `COMMON/STARTWAD.WAD` and binds `"sroid"` and `"light"` against a
global scene root. They are then re-bound per race with the per-track prop entry.
Loading `STARTWAD.WAD` unblocks the gantry, the gate and the rescue craft together.

**Item 1, the maintenance craft — half answered.** The model is
`COMMON/RESCU.PRM`, confirmed: one object, 51 polygons, whose name is `"grid2"`.
It occupies slot `[0x1e]` of the prop pool, so its pointer is `0x800a5158`. Exactly
one function reads it, `maybe_AnimateRescueCraftGlow` (`0x80048d08`), which animates
its **lights only** — per-vertex RGB on gouraud primitives from three fixed sine
phases. Nothing in it moves the craft, so the descent-and-departure path is still
unlocated.

**Item 3 gained a second purpose.** `sroid` is not only the visible start/finish
marker: per tick it is passed to `maybe_DetectShipCheckpointCollision`
(`0x80049508`), so the gate object *is* the lap-detection trigger. Drawing it and
detecting laps come from the same data.

**Item 2's formula is known but its input is not.** Each of the three lights is
placed at a track section's centre raised `0xbb8`, yawed along
`ratan2(section - section.Previous)`, with its node forced visible. Which three
sections is still open: `maybe_StartLightPlacements` is in `.bss` and is read as
`base + i*4`, so its writer leaves no direct reference.

**Items 4 and 5 — the animator does no gating.** There is no visibility test in
`maybe_AnimateSmokeTextureFrames` at all, so plumes appearing where retail hides
them are gated by the node `+0x40` visible flag or by the binder, not the animator.
Two concrete bugs in our version did surface:

- **Only the first primitive of each smoke object is restamped per frame.** The
  inner loop runs exactly once (`i = 0; do {...} while (i <= 0)`). We stamp every
  panel.
- **The frame advance is `if (tick % div == 0) frame += step`**, divisor and step
  being arguments, not a fixed rate. Both sets use divisor 1; fast steps 2, and the
  slow set's step is a register value rather than a constant.

The atlas base is also a global rather than 0, the object counts come from the
binder's out-params, and the RGB passed in is copied from a data address — smoke is
tinted, not white.

**The `0x40` face flag should be treated as unidentified.** Both of its consumers
are vestigial: `maybe_FindSectionByDifficulty` locates the start-line section and
then discards it in a self-branching delay loop, and `sub_8004c108`'s only side
effect is storing zero to one byte. Neither result reaches rendering or physics, so
`TrackFaceAlternateRoute` overstates what is known. The load-time dark red colour is
real; the meaning is not.

---

# The maintenance craft and the ship's start height

The craft visibly carries the player's ship down onto its pad while the intro
camera sweeps, then leaves during the countdown. Two things are now settled about
how that is *not* implemented in retail:

**The countdown does not move the ship.** `maybe_UpdateRaceStartCountdown`
(`0x800251d0`) only decrements the timer at ship`+0xe0` from `0xa6` and fires SFX at
counts 125, 83, 41 and 0. There is no position write anywhere in it.

**The ship is placed 300 units *above* its pad.**
`InitializeRaceShipsAndStartingGrid` sets position to the midpoint of the pad face's
vertices 0 and 2 plus `(normal * 0x4b) >> 0xa`. Normals are 4096-scaled, so on flat
road that term is `4096 * 75 / 1024` = **300 world units** of clearance, not a few.
Measured on TRACK01 slot 14: pad vertices at Y 500, normal Y -4096, ship Y 200.

So the descent **is** simulated. The ship starts hovering 300 up with no velocity,
and gravity lowers it onto the pad while the countdown runs. The port already
reproduces this, because it uses the same placement formula and runs the same
physics.

The maintenance craft does **not** descend with it. Observed behaviour: the craft
holds a position above the pad, the ship drops away from it, and only then does the
craft fly off. So its animation is three phases -- a stationary hover above the
ship's spawn point, a release, and a departure -- rather than a carried descent.
That means its height is independent of the ship's fall and sits somewhere above
+300, and the release is presumably timed against the countdown at ship`+0xe0`
rather than against the ship's position.

The path remains unlocated. `maybe_AnimateRescueCraftGlow` (`0x80048d08`) is the only
reader of the craft's object pointer and it only rewrites primitive colours, so
whatever sets its node translation is elsewhere. The departure is the most findable
part: it should be a per-tick position update gated on the countdown value.

## Correction

An earlier version of this section claimed the ship "is placed on its pad from the
start", with "a few units of clearance, not a hover", and that there was "no
unexplained spawn elevation". The first two are wrong: the offset is 300 units, a
real hover. The startup log line `spawn contact distance=300.0` was reporting that
clearance, and dismissing it as a mere probe artefact was a mistake -- the number
was meaningful and matched the placement offset exactly.

## Search for the craft's animator: what has been ruled out

The drop happens during the **intro camera sweep**, before the countdown proper. The
sweep is `UpdateRaceStartCameraArc` (`0x800209f4`), driven by the ship's own timer at
`+0xe0` running `0xa6` down to `0x64` -- about 66 ticks, 2.6 s at 25 Hz -- after which
it installs `UpdateChaseCameraFollow` or `UpdateCockpitCameraView` depending on
ship`+0x2a`.

Places checked that do **not** touch the craft or the ship's position:

| candidate | what it actually does |
|---|---|
| `maybe_UpdateRaceStartCountdown` (`0x800251d0`) | decrements `+0xe0`, fires SFX at 125/83/41/0. No position writes. |
| `UpdateRaceStartCameraArc` (`0x800209f4`) | interpolates the *camera* from a node 10 `Next` links ahead, eased quadratically on `0xc8 - ship->0xe0`, offset `-0x320` in Y. Reads the ship, writes only camera state. |
| `maybe_AnimateRescueCraftGlow` (`0x80048d08`) | the only reader of the craft's object pointer; rewrites primitive RGB only. |
| `maybe_InitRaceCameraAndShipNodeHierarchy` (`0x800699cc`) | resets the 15 ship nodes to identity and marks them visible. No craft, no parenting. |
| countdown gate globals `g_80094c90`, `g_800949b4` | read only by the countdown itself and `maybe_UpdateStartLightGantry`. |

So the craft's position update is none of these. Remaining ideas, untried: scan for
writes to `*(maybe_RescueCraftObject + 0x30)`'s translation through
`maybe_SetNodeTranslation` callers; or look for a per-tick handler in
`maybe_RaceMain` that is gated on ship`+0xe0` being above `0x64`, since that is the
window the drop occupies.

## `ship+0xe0` is a general-purpose timer, and `0x1f4` is the rescue hold

`maybe_IntegrateShipPhysicsFromPadInput` (`0x80066dac`) contains a hold, but keyed to
`0x1f4` (500) rather than the race-start values:

```asm
lw    $v1, 224($s0)     ; ship->0xe0
li    $v0, 0x1f4
bne   $v1, $v0, skip    ; only when it is exactly 500
      ...               ; walks Previous x4 then Next, rewrites ship->0x04 (section)
sw    $zero, 80($s0)    ; velocity X
sw    $zero, 84($s0)    ; velocity Y
sw    $zero, 88($s0)    ; velocity Z
addiu $v0, $v0, -1
sw    $v0, 224($s0)
```

So `ship+0xe0` is not solely the start countdown: it is a general ship timer that
takes `0xa6` at a race start and `0x1f4` for a **mid-race rescue**, which is what
`RESCU.PRM` is actually named for -- the craft that retrieves a wrecked ship and
replaces it on the track. The section-link walk here is the reposition.

Because the branch tests equality, the velocity zeroing happens on exactly one tick
and the timer then falls out of the window. That is a reset, not a sustained hold, so
it does not explain the ship being held at +300 through the intro sweep.

Observed timeline to account for, from watching retail:

| `ship+0xe0` | what is seen |
|---|---|
| `0xa6` → `0x64` | intro camera sweep; craft hovers holding the ship |
| `0x64` | **the ship is released** -- also exactly where `UpdateRaceStartCameraArc` installs the chase or cockpit camera |
| `0x64` → `0` | countdown; the craft is still present |
| `0` | the craft leaves |

The release coinciding with the camera handover at `0x64` is a strong hint that one
condition drives both. The player's physics must be suppressed above `0x64`, or the
ship would fall from +300 immediately; that suppression has not been located and is
not the `0x1f4` path above.

## Correction: the craft's animation phase is an accumulator

`maybe_AnimateRescueCraftGlow` was described above as using three *fixed* sine phases
(`0x8c`, `0x4b`, `0x20`). That was wrong. The disassembly is a read-modify-write:

```asm
lhu   $v0, 938($gp)      ; g_80094cba
addiu $v0, $v0, 140      ; += 0x8c
sh    $v0, 938($gp)
jal   GetSin
sra   $v0, $v0, 5        ; >> 5
addiu $v0, $v0, 128      ; + 0x80
```

so the phase advances **0x8c per tick** and the samples oscillate. BN's HLIL rendered
the store as `g_80094cba = 0x8c` and it was read as a constant assignment.

That rate is close to the `0xc8` per tick that `maybe_AimAndBobCameraObjects` uses for
its trackside camera bob, and the craft is observed to bob vertically in play. So the
function may well drive position as well as primitive colour, and the name is too
narrow. Checking whether it writes the node translation is now the first thing to do
rather than assuming, as was assumed here, that it is colour only.

## Port bug: the player's ship bobs after landing

Distinct from the maintenance craft, and observed in the port rather than retail: after
the ship is released it oscillates vertically instead of settling.

What is established:

- The ship spawns at **+300** above the pad, from retail's own
  `(normal * 0x4b) >> 0xa` with a 4096-scaled normal.
- The surface spring's normal term reaches zero at **256**
  (`trackSurfaceZeroNormalForceDistance`, from `16384/256 == 64`).

So a 44-unit settle from 300 down to the 256 equilibrium is expected and correct. A
*sustained* bob is not: it means the spring is returning to equilibrium without
enough damping and overshooting each time, so energy is not being removed.

Where to look: `IntegrateShipPhysicsAndTrackContact` around `0x80030e70`, which is
where the 75-unit divisor floor (`trackSurfaceMinimumDistance`) comes from. The
damping term that should sit alongside the spring has not been checked against the
executable, and the port's version may be missing it or applying it at the wrong
magnitude. Note the chase camera has a comparable pair -- a `/64` spring and a `/8`
decay -- so the surface spring plausibly has its own decay divisor that was never
ported.

This is worth fixing before the maintenance craft: a visibly oscillating ship will
make any craft animation impossible to judge.


## The rescue craft's lights are fully recovered

Verified against play: the craft's rear lights are red and blink on and off as it
departs. `maybe_AnimateRescueCraftGlow` (`0x80048d08`) produces exactly that.

One accumulating phase drives three sine samples:

```c
phase += 0x8c                                       // per tick
s1 = clamp((GetSin(phase)        >> 5) + 0x80, 0xff)
s0 = clamp((GetSin(phase + 75)   >> 5) + 0x80, 0xff)   // 75/4096 out of step
t2 = clamp((GetSin(phase/2 + 32) >> 5) + 0x80, 0xff)   // half rate
```

and the eleven primitives split into three groups, written R, G, B:

| primitives | colour | effect |
|---|---|---|
| 0–1 | `(0x28, s0, 0x28)` | green, pulsing |
| 2–5 | `(t2>>1, t2>>1, t2)` | blue, pulsing at half rate |
| 6–10 | `(s1, 0x28, 0x28)` | **red, pulsing** |

`(sin >> 5) + 0x80` sweeps the full 0 to 0xff, so these blink rather than glow. The
last group being red is the agreement that makes the grouping verified rather than
inferred: the code says the final five primitives pulse red, and the screen shows red
blinking rear lights.

This is enough to render the craft correctly once it can be positioned. The motion
remains the only missing piece, and the eliminated candidates are listed above.

## Static search for the craft's mover: exhausted

Tracing from the asset to whatever manipulates it is the right instinct, and it is what
was tried. The chain is: `maybe_TrackPropPool` (`0x800a50e0`) slot `0x1e` holds the
object pointer at `maybe_RescueCraftObject` (`0x800a5158`); the object's node is
`*(obj + 0x30)`; the node's translation is at `+0x14`, `+0x18`, `+0x1c` with `+0x40`
set to 1, as `maybe_SetNodeTranslation` (`0x8001e090`) shows.

Every static route from there has been followed and none reaches it:

| approach | result |
|---|---|
| code references to `0x800a5158` | one, `maybe_AnimateRescueCraftGlow`, colour only |
| all 40 callers of `maybe_SetNodeTranslation` | none passes the craft's node |
| functions receiving the whole pool (`maybe_DrawTrackBarrierMesh`, `maybe_DispatchShipDrawFunction`, `InitRaceCameraFromShip`) | none indexes slot `0x1e`; an apparent offset-120 access was a `$sp` frame save |
| the countdown gate globals | read only by the lights and the gantry |
| every animator called from `maybe_UpdatePerTrackAnimatedObjects` | all accounted for, none is the craft |
| inline writes of the `+0x14`/`+0x18` translation pair | 139 functions, since offset 20 is common to many structs -- not selective enough to use |

The structural reason this is hard: the node is parented into the scene tree at
`g_800bde0c`, and the pool is indexable, so anything reaching the craft through a tree
walk or a computed index leaves no reference to either the object pointer or the slot.

## What would settle it

A watchpoint, using the PCSX-Redux GDB connection the plugin already provides. With the
game paused while the craft is on screen:

1. read `*0x800a5158` for the object address
2. read `*(object + 0x30)` for its node
3. watch `node + 0x14` for writes
4. resume; the watchpoint fires inside whatever moves it

That is decisive where the static search is not, because it observes the write rather
than trying to predict which code performs it. It needs the emulator running at that
moment, so it is the next step to take together rather than something to derive from
the binary alone.

## SOLVED: the craft is moved by a waypoint path state machine

A watchpoint on the craft's node translation, set in an emulator, landed in
`0x80067xxx` -- a cluster I had named as AI ship heading. That naming was wrong, and it
is why every static search failed: I had already listed
`maybe_IntegrateAiShipHeading` among the 40 callers of `maybe_SetNodeTranslation` and
dismissed it on the strength of its own name and its three AI-named callers.

The write is explicit once you look:

```asm
lw $v0, 120($s1)      ; $s1 is the prop pool; +0x78 is slot 0x1e
lw $a1, 8($s0)        ; entity position x, y, z
lw $a0, 48($v0)       ; object + 0x30, its node
jal maybe_SetNodeTranslation
                      ; then a rotation matrix from the entity's angles
                      ; then node->0x40 = 1
```

Offset **120 is `0x1e * 4`**, the rescue craft's slot in `maybe_TrackPropPool`.

### The entity structure

Moving objects follow a linked list of waypoints, each carrying its own small state
machine:

| offset | field |
|---|---|
| `+0x00` | pointer to the current waypoint or track section |
| `+0x08` | position x, y, z |
| `+0x18` | velocity x, y, z |
| `+0x28` | acceleration x, y, z |
| `+0x38` | yaw, pitch, roll as halfwords |
| `+0x44` | phase timer, counted down per update |
| `+0x48` | pointer to the next state function |

States chain by writing `+0x48`, so the flight is a sequence of timed phases rather
than a scripted path. `maybe_IntegrateMovingObjectPath` does the integration --
velocity accumulates from acceleration, decays by 1/8, and applies to position at 1/64,
the same spring-and-decay shape as the chase camera -- then pushes the result into the
scene graph.

The phase thresholds in the first state are `0x190`, `0x1f4`, `0x2c6`, `0x302` and
`0x320`. That `0x1f4` is the same value `maybe_IntegrateShipPhysicsFromPadInput` tests
when it zeroes a ship's velocity and rewinds its section, so the rescue timers and the
mid-race rescue are one scheme.

### Corrected names

| was | now |
|---|---|
| `maybe_IntegrateAiShipHeading` | `maybe_IntegrateMovingObjectPath` |
| `maybe_UpdateAiShipHeadingTowardNextMarker` | `maybe_MovingObjectFlightStateA` |
| `maybe_UpdateAiShipHeadingTowardTrackMidpoint` | `maybe_MovingObjectFlightStateB` |
| `InitRaceCameraFromShip` | `maybe_InitMovingObjectPath` |
| `maybe_ResetAiShipHeadingChain` | the chain reset in the same cluster |

The lesson is narrow and worth keeping: a wrong name defeated a correct search. The
function was in the result set of exactly the right query and I filtered it out by
reading its label instead of its body.

## The stationary bob, measured

Confirmed by simulating a spawn with no input and logging height. The craft drops from
Y 200, overshoots to 312, and settles at 275 -- but takes **275 ticks, eleven seconds**,
with swings decaying 62.6, 38.6, 22.2, 13.4, 7.8, 4.6, 2.7, 1.6, 0.9. That is a ratio of
about 0.6 per swing, needing nine swings. Retail settles in around two.

The mechanism is in `integrateAirborneShipPhysics`:

```go
redirectTarget = |velocity| * Forward
accel.Y = (redirectTarget.Y - Velocity.Y)/springDenom + (thrust.Y + gravityBiasY)/InertiaFactor
springDenom = 4*brakeSum + 20
```

With speed, `redirectTarget` is large and aligned with `Forward`, whose Y component is
near zero on level track, so vertical velocity is pulled hard toward zero and the bob
stops. Stationary, `|velocity|` is near zero so there is no redirect at all, leaving only
the `1/20` term -- a retention of 0.95 per tick. That is exactly the observed behaviour:
a sine wave while standing still, settling as soon as the craft moves.

So the weak damping is not a missing line; it falls out of the redirect being
proportional to speed. Two possibilities, and they need distinguishing before anything is
changed:

1. Retail has a separate damping term in its **track contact** response that the port
   never took across, independent of the airborne redirect. `trackSurfaceMinimumDistance`
   (75) and `trackSurfaceZeroNormalForceDistance` (256) came from
   `IntegrateShipPhysicsAndTrackContact` around 0x80030e70, but whatever multiplies the
   resulting normal force was not examined for a velocity term.
2. The port's surface spring is too strong, so it injects more energy per bounce than
   retail and the same damping cannot absorb it.

The way to tell them apart is to read the normal-force computation in
`IntegrateShipPhysicsAndTrackContact` and check whether it subtracts anything
proportional to the ship's velocity along the surface normal. A spring with no such term
is undamped by construction, and the 1/20 redirect is then the only thing removing
energy -- which matches the measured 0.6 ratio.

Also reported and not yet investigated: the craft **starts moving on its own after a
couple of seconds** with no input. Nothing in the measured run shows lateral drift, so
this may be a separate defect in the horizontal axes rather than a consequence of the
vertical oscillation.

## Runtime session: what it confirmed and what it added

A live DuckStation session (see `docs/duckstation-findings-rescue-craft.md`) set a write
breakpoint on the craft's node translation. It agrees with the static analysis at the
instruction level and adds two things static analysis could not reach.

**Confirmed.** The write PC is `0x8001E094`, inside `maybe_SetNodeTranslation`, and `ra`
is `0x800677C4` — exactly the return address of the `jal 0x8001e090` at `0x800677bc`
inside `maybe_IntegrateMovingObjectPath`. Two independent methods, same answer.

Also confirmed that `node + 0x40` **is** set through the standard helper rather than
bypassed, which was one of the two possibilities worth distinguishing. The craft goes
through the ordinary scene-graph update path.

**Added.** The entity struct is at **`0x800be420`**, now named
`maybe_MovingObjectState`. Static analysis had its layout but never its address, since
it is only ever reached through `arg1`.

The integrator's unidentified callee at `0x80025608` is `maybe_WrapAngleSigned`:
`a &= 0xfff; return a < 0x801 ? a : a - 0x1000`. A core helper called from ten places,
equivalent to the port's `game.Angle.Signed()`.

**One detail to reconcile.** The session reports the waypoint walk following a NEXT
pointer at `object + 0x4`, where `maybe_InitMovingObjectPath` walks `+0x8`. In a track
section `+0x4` is Previous and `+0x8` is Next, so either there are two walks in different
directions, or one of the two readings is off by four. Worth settling, since it decides
which way along the track a path runs.

**Flight altitude, as measured.** Y at departure is between −8385 and −9399, against
−423 at one earlier sample during the sweep. Negative Y being up, the craft **climbs
roughly 8000 units** as it leaves. X and Z at departure sit near −33000 and +60000, a
long way from the start line at roughly (−34755, −36318), so it travels a considerable
distance rather than fading out.

**What the session could not get, and why it matters.** No sample landed inside the
166-tick countdown window: the polling loop's round-trip latency meant every pause landed
after the countdown had already reached zero. So the entire light-phase and release
portion of the trajectory is unmeasured, and the six captured rows are all from the
departure. Getting the rest needs a logging breakpoint that records without pausing,
not a poll-and-sleep loop — which is a tooling question rather than an analysis one.
