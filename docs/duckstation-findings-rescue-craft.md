# Findings: rescue-craft investigation (live DuckStation session)

Answering `docs/duckstation-task-rescue-craft.md`. Short version: got a solid answer
on priority 1 (the writer function and its full call chain, including a real
waypoint-list walk), a partial and honestly-flawed answer on priority 2 (CSV), and
did not get to priority 3 at all. Details and caveats below — the gaps are as
informative as the hits, so they're called out rather than smoothed over.

## 1. The function that writes the craft's position

`OBJ` and `NODE` resolved as expected (`OBJ = 0x80109cb4` stayed stable across at
least one race restart; `NODE = 0x80109cf4`). A write breakpoint on `NODE + 0x14`
(`0x80109d08`, 4 bytes) fired at:

```
PC = 0x8001E094      sw a1, 0x14(a0)
```

This is inside a small generic helper starting at `0x8001e090`, which writes all
three axes plus the changed flag — call it `SetNodeTranslation(node, x, y, z)`:

```
0x8001e090:  <prologue>
0x8001e094:  sw a1, 0x14(a0)      ; node->x = x
0x8001e098:  sw a2, 0x18(a0)      ; node->y = y
0x8001e09c:  sw a3, 0x1c(a0)      ; node->z = z
...
             sh v0, 0x40(a0)      ; node->changed = 1   (delay slot after jr ra)
             jr ra
```

This is the same helper the rest of the scene graph uses for any node move — not
something specific to the rescue craft.

Following `ra` (`0x800677C4`) up the call chain:

- The immediate caller stages a position into a **generic scratch struct at
  `0x800be420`**: offsets `+0x8/+0xc/+0x10` hold the position, `+0x38/+0x3a/+0x3c`
  hold a 16-bit Euler rotation. It calls `SetNodeTranslation` with the position and
  a separate helper at `0x8001e458` (`BuildRotationMatrixFromEulerAngles`) with the
  rotation. So the craft's transform is fully computed off to the side before being
  applied to the node in one shot.

- That scratch struct is filled in by a **spring/velocity-damped integrator at
  `0x800676D4`**. It uses the same `sra`-by-3-then-6 decay pattern already ported
  into this codebase's camera-spring code (see `internal/render/camera.go`), so this
  reads as the *general* PS1 spring-follow primitive, reused here for the craft
  rather than something craft-specific. It also calls an unidentified function at
  `0x80025608` — not chased further.

- The integrator is called from an **outer function** (starts at or before
  `0x80067640`) that first walks a **linked list of waypoint nodes** before calling
  the integrator: it checks bit 0 of a flags field at `object+0x96`, and follows a
  NEXT pointer at `object+0x4` to advance. This is a genuine path-follow system —
  the craft has a sequence of waypoint targets and springs toward whichever one is
  currently active, not a fixed hover point and not a scripted keyframe tween.

**Confirms one of the task's open questions directly**: `NODE + 0x40` *is* being set
to 1 by the engine's own `SetNodeTranslation` helper, in the same instruction group
as the position write (delay-slot `sh` after `jr ra`). The "position moves but flag
stays 0" (direct-write) scenario did **not** happen — the craft goes through the
standard node-update path like everything else.

## 2. The craft's flight path (CSV) — incomplete, with a real gap

This is the weak part of the session and I want to be upfront about it rather than
present partial data as if it were the requested range.

**What was asked for**: samples spanning race load → ~10s after countdown hits 0,
covering the 166→100 intro sweep, 100→83 release, 83→42 red, 41→1 amber, 0 green,
and the departure.

**What was actually captured**: every sample taken during the CSV-sampling loop
read `0x800949b4 == 0` (countdown already at zero) at the moment of the pause. The
loop was `continue()` → sleep ~0.4–0.6s → `pause()` → read countdown + position, and
in every iteration the round-trip latency meant the countdown had already run out
before the pause landed — the light-sequence and pre-release phases were missed
every single time, not just once. I did not find a way to reliably land a pause
inside that 166-frame (~6.6s) window with this polling approach; a real fix would
need an actual conditional/read breakpoint on the countdown register transitioning
past specific values, not manual polling, and that wasn't attempted before the
session was cut short.

So the only real, decoded data is six samples, all at `countdown = 0`, i.e. all from
the **departure phase**, from two separate race attempts:

```
countdown,x,y,z
0,-33038,-8385,63489
0,-35387,-9399,63657
0,-31810,-8412,60214
0,-34567,-8385,58577
0,-35313,-8385,63519
0,-35086,-9195,58984
```

(Y is negative throughout, consistent with "negative Y is up" and the craft hovering
well above the pad.) These aren't in strict time order across the two attempts (the
race was restarted between some of them), so treat this as scattered snapshots of
the departure region, not a continuous trajectory — X and Z both swing across a
~5000-unit range sample to sample, which just reflects the craft actively flying
away, not noise.

One earlier data point, from the very first pause (during the intro sweep, engine's
own on-screen timer showing "0:50.0", *before* the write breakpoint was armed): the
breakpoint-hit register dump gave `a1/a2/a3 = X=-32155, Y=-423, Z=-25920` at the
write. I did not cross-check `0x800949b4`'s exact value at that same instant, so I
can't place it precisely on the 166→0 scale — it's suggestive that Y was much
closer to zero (less "up") earlier in the sequence and grew to ~-8400..-9400 by the
time of departure, i.e. the craft climbs during/after release, but that's one
data point, not a trend line.

**Bottom line: the pre-countdown/light-phase portion of the CSV was never
captured.** If this data still matters, it needs a proper breakpoint-driven capture
(e.g., a read/write watch that logs on every write to `NODE+0x14..0x1c` rather than
polling on a sleep timer) rather than a repeat of this approach.

## 3. Player ship position comparison — not attempted

Ran out of runway before getting to this. `0x80095858` (ship array pointer) →
`+0x40/0x44/0x48` for the player's position was never read. No data either way on
whether the player starts ~300 units above the pad and falls while the craft holds
its altitude — that comparison is still open.

## 4. Anything that contradicts expectations

Nothing found contradicts the task doc's model. The one confirmed fact
(`NODE+0x40` is set through the standard helper, not bypassed) matches the
"expected, but worth checking" case rather than the "surprising" one. The
waypoint-linked-list walk (bit 0 of `object+0x96`, NEXT at `object+0x4`) wasn't
something the task doc predicted one way or the other, so it's a genuine addition:
the craft's motion is target-list-driven, not just "spring toward one fixed
external point."

## Key addresses for follow-up

| Symbol | Address | Notes |
|---|---|---|
| `OBJ` | `0x80109cb4` | rescue craft object pointer, stable across restart |
| `NODE` | `0x80109cf4` | scene node, `OBJ+0x30` |
| write PC | `0x8001E094` | `sw a1,0x14(a0)` inside `SetNodeTranslation` |
| `SetNodeTranslation` | `0x8001e090` | generic node position+flag helper |
| `BuildRotationMatrixFromEulerAngles` | `0x8001e458` | generic node rotation helper |
| scratch stage struct | `0x800be420` | pos `+0x8/+0xc/+0x10`, rot `+0x38/+0x3a/+0x3c` |
| spring integrator | `0x800676D4` | decay pattern matches ported camera spring |
| unidentified callee | `0x80025608` | called from the integrator, not chased |
| outer waypoint-walk fn | `≤0x80067640` | checks `object+0x96` bit 0, follows `object+0x4` |
| countdown mirror | `0x800949b4` | 166→0, confirmed all captured samples landed at 0 |

## Suggested next step, if this gets picked back up

A logging write-breakpoint (log-and-continue on every hit to `NODE+0x14`, rather
than pause-sample-resume) would get the full 166→0 trace in one race, and would
also settle the object+0x96/waypoint-list question by dumping the waypoint list
itself when the outer function is entered. Didn't attempt this because the DuckStation
MCP breakpoint tool as used here paused on hit rather than logging-and-continuing;
worth checking whether it supports a non-pausing/logging mode before trying again.
