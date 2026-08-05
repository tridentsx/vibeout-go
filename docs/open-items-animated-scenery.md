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
