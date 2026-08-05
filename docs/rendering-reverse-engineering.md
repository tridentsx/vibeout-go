# Rendering reverse-engineering notes

Findings from live/static analysis of `SLES_003.27` (via the recovered Binary
Ninja database and live DuckStation debugging) toward understanding
animated-object rendering (signs, start lights, smoke, fans) that this port
doesn't reproduce yet. Status markers: **confirmed** (verified against the
binary and/or live memory), **hypothesis** (plausible, not verified),
**ruled out** (checked and disproven).

## Tooling available for this kind of work

- Binary Ninja recovered DB `sles_003.27.recovered.bndb`, via the `binassist`
  MCP tools (`get_code` for decompile/disasm, `search_functions_by_name`,
  `search_strings`, `xrefs`, `get_data_at`).
- Live DuckStation session via the `duckstation` MCP tools: `read_memory`/
  `write_memory` (patch RAM directly), `dump_vram`/`read_vram_region` (raw
  16-bit VRAM, PNG or binary), `vram_watch` (write-breakpoint on a VRAM
  rectangle, reports hit PC), `breakpoint` (execute/read/write breakpoints
  on CPU addresses), `memory_watch`, `press_button`/`release_button`/
  `input_sequence` (drive the game), `save_state`/`load_state`,
  `frame_step`/`pause`/`continue`, `take_screenshot`.
- **Caveat on live memory reads**: several fixed addresses cited in the
  recovered DB read back as zero or unrelated data when checked live
  (`0x800d8a98`, `0x800c44ca`, `0x8009495c` all did). PS1 games commonly
  reuse the same RAM regions for different overlays/subsystems at different
  times, and some of Binary Ninja's cross-references turned out to be
  incidental reuse of a scratch address by unrelated code (menu screens,
  ship-trail rendering, `main()`), not a dedicated single-purpose variable.
  Don't trust a static address's "meaning" until confirmed live in the
  specific game state you care about.

## Track identity

Confirmed: a track display-name table lives at `0x80018980`, 4-byte-aligned
NUL-terminated strings, index order matching menu/display order:

```
0: TALON'S REACH
1: GARE D'EUROPA
2: VOSTOK ISLAND
3: SPILSKINANKE
4: SAGARMATHA
5: VALPARAISO
6: ODESSA KEYS
7: PHENITIA PARK
```

This disc's `TRACK01` folder = index 0 = Talon's Reach (independently
confirmed earlier via the FSOL billboard texture match). The per-track
special-effects dispatch below uses a *different* numbering that doesn't
line up 1:1 with this table (see below) — likely a raw disc/folder-adjacent
index rather than the display-order index.

## Per-track special-effects dispatch (confirmed structure, mapping unclear)

`maybe_RaceMain` (`0x8003f494`) contains a long chain of
`if (trackIndexField == N) { load "name" resources }` blocks, keyed off a
byte read from the race-setup struct (`arg1[5].b`). Each block loads named
sub-resources via `maybe_LoadIndexedResource(handle, 0, addr, "name", count,
&outVar)`:

| index | resources loaded |
|---|---|
| 1 | `"smokes"`, `"smokef"`, `"redb"`, plus `smoke.cmp`/`alphared.cmp` |
| 6 | `"AGunit"`, `"camera"`, `"redb"`, `"screen"`, `zetared.cmp` |
| 8 | `"AGunit"`, `"camera"`, `"redb"`, `alphared.cmp` |
| 0x11 (17) | `"dish"`, `"camera"`, `"grid1"`, `"redb"`, `upsilred.cmp`/`rhored.cmp` |
| 7 | `"screen"`, `"redb"`, `"AGunit"`, `etared.cmp` |
| 0xd (13) | `"torch"`, `fire.cmp`, `"redb"`, `nured.cmp` |
| 2 | `"train"` (+`COMMON/TRAIN.PRM`/`.CMP`, confirmed present on disc), `"camera"`, `"zeppelin"`, `"stewy"`, `"pylon"` |

Checked: TRACK01's own `TRACK.WAD` contains only the standard
`track.tr{v,f,s}`/`library.*`/`scene.*`/`sky.*` members — none of the named
resources above. `COMMON/STARTWAD.WAD` also doesn't contain them. Wherever
these come from, it isn't a WAD this disc extraction has decoded yet.
Talon's Reach (table index 0) doesn't match any of the `if` conditions
above, so **the per-track hazard/particle system is not what's producing
the smoke seen on Talon's Reach** (confirmed present via screenshot) — that
must be a separate, more generic per-object emitter, not yet found.

Real particle-system functions exist and are confirmed genuine (not
speculation): `maybe_SpawnTrackEnvironmentParticles`,
`maybe_InitTrackEnvironmentParticlePresetA`..`E` (`0x8006b028`-`0x8006ba5c`),
`maybe_UpdateActiveParticleSlots` (`0x800269a4`). A real `TEXTURES/SMOKE.CMP`
and `TEXTURES/FIRE.CMP` exist on disc (paths outside individual track
folders). This confirms the particle system is real infrastructure; what's
unconfirmed is which code path feeds it for a track (like Talon's Reach)
that isn't one of the special-cased indices above.

## Animated billboard signs (FSOL / Red Bull / Energy Drink cycling)

Confirmed live: the billboard's displayed content changes over time while
sitting still at the same physical location (screenshots ~30s apart showed
"Red Bull" then "Energy Drink" then back to "Red Bull").

`maybe_UpdatePaletteAnimation` (`0x800221f4`) does CLUT-style color writes:
reads a 12-bit phase counter (`gp+140`, +=128/call, masked `&0xfff`), and on
each call does a **plain CPU memory store** (`sh`, not a GPU command) of 4
new 16-bit color values, each computed as `*(0x800c44ca + a3*8)` (a small
fixed source-color table) into `*((clutSlotIndex<<3) + *(t0+0x14) + 2)`,
where `t0 = *0x800a50f0` and the slot indices come from an array at
`0x800d8a98`.

Per the PSYQ CLUT-cycling pattern (`LoadImage()` + `DrawSync()` uploads a
RAM-side palette array to its VRAM CLUT rectangle each cycle), a raw CPU
store like this must be writing a RAM-side mirror that gets bulk-uploaded to
VRAM separately (likely batched into the same per-frame ordering-table
flush as everything else) — **not** a direct VRAM poke. This is consistent
with PS1 VRAM being reachable only through GPU commands.

**Tested live and inconclusive**: NOP'd the `jal 0x800221f4` call site
(`0x80041064` in `maybe_RaceMain`, confirmed via reading the live
instruction bytes: `7d 88 00 0c` = `jal 0x800221f4`, delay slot already a
NOP). Observed **no change** to sign cycling, start lights, or fan rotation
— but the gating flag checked immediately before the call (`lbu` from
`0x8009495c`, `beq` skips the call if zero) read **0** in the live session,
meaning **the call was already being skipped** before the patch — this was
not a valid test of the function's effect. The flag itself turned out to be
a generic scratch address reused by unrelated subsystems (see caveat
above), so tracing its setter didn't lead anywhere useful. Patch was
reverted (original bytes restored and confirmed via read-back).

**Open**: whether `maybe_UpdatePaletteAnimation` is actually responsible for
the sign cycling remains unconfirmed. Next step would be a `breakpoint
type=write` (not `vram_watch` — see below) on the actual RAM mirror once its
live address is known, or catching the gating flag going non-zero live to
learn when/why this path activates.

## Start lights (red → yellow → green → chase pattern)

Confirmed live via DuckStation:
- The light bar's rendered pixel colors are near-pure primaries:
  red ≈ (248,0,0), green ≈ (0,248,0) — real 5-bit-per-channel palette
  entries, not lighting/blend artifacts.
- **Ruled out**: `COMMON/LIGHT.PRM`/`LIGHT.CMP` (also duplicated inside
  `STARTWAD.WAD`, byte-identical) is a generic gray lamppost fixture —
  confirmed via direct polygon/CLUT dump, no color in its palette at all.
  Not the light-gantry model.
- **Ruled out**: none of TRACK01's 39 `SCENE.CMP` textures contain baked-in
  red or green pixels (scanned all of them programmatically). So either the
  color comes from flat per-polygon face color (no texture), or from a
  shared grayscale texture whose CLUT gets swapped at draw time (multiple
  pre-loaded CLUT blocks, only the primitive's `.clut` field selection
  changes — no VRAM rewrite needed at all; this is a real, documented PSYQ
  technique).
- Raw VRAM diff between a red-phase and a green-phase pause (excluding the
  two display-buffer rectangles) found **zero byte differences** in the
  texture-atlas region of VRAM. This is consistent with the "swap which
  CLUT is referenced" hypothesis (nothing in VRAM needs to change) and
  inconsistent with "rewrite CLUT contents in place" for whatever's
  producing this specific effect.
- The audio side is confirmed separately: `maybe_UpdateRaceStartCountdown`
  (`0x800251d0`) plays the 3-2-1 beep sequence off a countdown timer at
  fixed thresholds (`0x53`, `0x7d`, `0xdc`).

**Not yet found**: the actual code that selects/writes the light color per
countdown phase. `maybe_UpdatePaletteAnimation`'s gating flag was 0 during
normal play (see above), so it's not confirmed to be involved.

**Best next step** (not yet attempted): find the live RAM address of
TRACK01's loaded `SCENE.PRM` light-bar object data (trace the scene loader's
heap allocation), then set a `breakpoint type=write` there — this pauses
execution and reports the exact PC, which is far more reliable than
guessing function names or diffing VRAM. Coordinating the timing live with
a human watching the screen (rather than blind screenshot polling) would
also help a lot, since the color phases only last ~1 second each.

## Scene/node hierarchy (confirmed, general architecture)

`sub_800699d0` (called from `maybe_InitRaceCameraAndShipNodeHierarchy`,
`0x800699cc`) initializes 15 node slots starting at `0x800a50e0` (matches
the ship/camera node array passed to `InitRaceCameraFromShip`), each with an
identity transform at node `+0x30` (and `+0x30+0x20`), confirming a
standard PSYQ-style scene-graph: per-node position/rotation matrices, a
matrix stack (`PushMatrix`/`PopMatrix`/`ApplyMatrix`/`SetRotMatrix`/
`SetTransMatrix`, all present with real PSYQ names in this recovered DB).

Also built here: a 128-entry sine-derived byte table at `0x800a5010`
(`v = GetSin(i<<4); table[i] = ((v<<1)+v)<<2 >> 12) + 0x14`). **Checked its
only consumer** (`sub_80069d88`, 5 call sites all within one function): this
turned out to be the **loading-screen** draw function (draws "LOADING",
class-select text like "VECTOR"/"VENOM"/"RAPIER"/"PHANTOM", "AG SYSTEMS"),
not a general per-object animation table. Likely drives a loading-screen
spinner/pulse effect. Not useful for fans/flags as hoped — ruled out as the
general animation mechanism.

## Per-frame render dispatch (confirmed live, partially unresolved)

`maybe_RaceMain`'s normal in-race path (reached when
`*0x800ea570 == 0xffffffff`, i.e. no pause/menu overlay active) is
surprisingly short:

```
maybe_UpdateScreenTransitionCounter()
maybe_DispatchShipDrawFunction(*0x80095858, 0, 0x800a50e0, 0)
(*0x800be468)(0x800be420, *0x80095858, 0x800a50e0)
return maybe_CallShipEffectCallback(0x80090000)  // tailcall
```

No explicit "draw track scenery" call is visible here — scenery submission
must happen through the indirect call `(*0x800be468)(...)` or elsewhere
entirely (possibly outside `RaceMain`, in whatever outer loop calls it).

**Confirmed live**: read `0x800be468` from the running game (mid-race) —
currently holds `0x80067340`. That address decodes to valid MIPS code
(`addiu $sp,$sp,-72`, a real function prologue), **but Binary Ninja's static
analysis never discovered it as a function** (only reachable via this
indirect pointer, so `get_code`/`analyze_function` both fail on it — "not
found"). This is the most promising unexplored thread for understanding the
*actual* per-frame scenery/ship draw pipeline, but needs either (a) forcing
Binary Ninja to analyze a function at that address (not available through
the current MCP tool surface — no "create function at address" call found),
or (b) hand-disassembling from raw bytes via `get_data_at`, or (c) setting a
live `breakpoint type=execute` at `0x80067340` and single-stepping.

## Practical implication for this Go port

None of the animated-object systems above (signs, lights, smoke, fans) are
implemented yet. Of the four, the sign-cycling and start-light mechanisms
both plausibly reduce to "which CLUT/texture-frame is referenced," not
runtime pixel rewriting — which would be relatively cheap to port once the
actual trigger condition and frame set are known (swap `polygon.Texture`
based on a track-local animation-state counter, similar to how
`objectPresentation` already resolves textures). Fans and general
per-object rotation remain completely unexplored — no lead found yet at all
(the sine-table lead was a false trail, see above).
