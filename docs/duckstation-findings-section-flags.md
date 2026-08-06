# Findings: section flag bit 0x01 (single-pause bulk read)

Answering `docs/duckstation-task-section-flags.md`. This confirms, for a third time
and by three different methods across two sessions, the same result.

## The one question that matters

**Does any track section have bit `0x01` set in its flag halfword? No — 0 of 321.**

At a single paused frame (mid-race, Talon's Reach, `countdown == 0`, i.e. well past
the point the task doc says the bit should already matter if anything sets it):

- `TRACKDATA` (`0x8009553c`) = `0x801fd3c0`
- `SECTIONS` (`TRACKDATA + 0x14`) = `0x801d0a0c`
- Bulk-read all `321 * 156 = 50076` bytes from `SECTIONS` in one call and checked the
  halfword at `+i*156 + 0x96` for every section.

Result: **0 sections have bit 0x01 set.** Full value distribution across all 321
flag halfwords:

| value | count |
|---|---|
| `0x00` | 289 |
| `0x20` | 30 |
| `0x28` | 1 |
| `0x30` | 1 |

No other values appear. This exactly matches what static analysis found on-disc
(0/0x20/0x28/0x30 only), and matches an earlier bulk read from the same race taken
during the intro sweep (before the countdown ran).

## Corroborating evidence from a separate live session

Before this task doc existed, a related investigation (see
`docs/duckstation-findings-rescue-craft.md`, §5) had already:

1. Bulk-read the same 321-section array while paused during the intro sweep — 0/321
   set, identical distribution.
2. Snapshotted that same 50,076-byte region at that point and diffed it after
   letting the race run all the way through the full light sequence to
   `countdown == 0` — **zero bytes changed anywhere in the region**, not just the
   flag bit.

Combined with this task's independent bulk read at a third point in time (after the
craft had already launched), that's three separate reads spanning the entire race-
start sequence, two of them bracketing a full run with a byte-level diff in between,
all agreeing: nothing ever sets bit 0x01 on any section, for this track, across the
whole sequence. This is about as settled as a negative result gets without full
static coverage of every code path that could theoretically write to `+0x96`.

**Conclusion: `maybe_InitMovingObjectPath`'s waypoint-list walk is not steered by a
runtime flag write. It runs to whatever bound it hits.**

## The two extra words, from the same paused frame

- `0x800949b4` (countdown) = `0` — this pause landed after launch, not during the
  countdown itself (the paused frame was left over from a prior investigation in the
  same session, not freshly set up for this task).
- `0x800a5158` (craft object pointer) = `0x80109cb4` (non-zero, valid, same value
  seen in every earlier read this session — stable across restarts).
  - `+0x30` (node) = `0x80109cf4`
  - node position (`+0x14/+0x18/+0x1c`): **X = -34399, Y = -8385, Z = 58562**

Nothing read as zero or nonsense; every pointer resolved as expected.

## Coarse path checkpoints — skipped

The task doc explicitly says to skip this if it means fighting the emulator. Given
this session's earlier experience in the rescue-craft investigation (per-frame
breakpoint stepping is accurate but slow; sleep-based jumps overshoot by roughly
100x because `continue()` runs far faster than real time under this MCP), landing
cleanly on seven specific countdown values without either fighting latency or
disturbing whoever is at the controls wasn't attempted here. See
`docs/duckstation-findings-rescue-craft.md` §5 for a proposed non-pausing
`memory_watch`-polling approach if this is still wanted.
