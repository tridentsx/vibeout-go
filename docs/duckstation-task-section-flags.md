# DuckStation task: one paused frame, bulk reads

WipeOut 2097 PAL (SLES_003.27).

**Design constraint:** driving a realtime emulator through an LLM is expensive — every
round trip has latency the game does not wait for, and menu screens time out before a
response lands. So this task is built to need **one pause and zero menu navigation**.
The human gets the game into a race by whatever means is easiest (a save state is
ideal); everything after that is memory reads from a single paused frame.

If you find yourself trying to navigate menus, stop and ask the human to do it.

## Setup, done by the human not the LLM

Get to a point **during a race on Talon's Reach** — any moment after the countdown
starts is fine, the start sequence is not required. Then pause the emulator.

## The one question that matters

Does any track section have bit `0x01` set in its flag halfword?

This decides where the game's moving-object path system begins, and it cannot be
answered from the disc: no section has that bit on disc, yet the code walks the section
list looking for it. Either something sets it at runtime, or that walk runs to its bound.

### How to read it

All addresses are KSEG0 (`0x800xxxxx`). If DuckStation wants physical addresses, drop
the `8`: `0x8009553c` becomes `0x0009553c`. Everything is little-endian.

1. Read the 32-bit word at `0x8009553c`. Call it `TRACKDATA`.
2. Read the 32-bit word at `TRACKDATA + 0x14`. Call it `SECTIONS`.
3. Sections are **156 bytes** each. Talon's Reach has **321**.
4. **Dump `SECTIONS` through `SECTIONS + 50076`** (321 × 156) as one bulk memory read if
   your tooling allows it, and report the raw bytes or a file. That is a single read and
   I can extract everything from it myself.

If a bulk dump is not possible, then read just the halfword at `SECTIONS + i*156 + 0x96`
for each of the 321 sections and report the 321 values. Please do not sample a subset:
the interesting case may be a single section.

## Two extra words, from the same paused frame

Cheap to add since you are already paused:

- `0x800949b4` — 32-bit, the countdown mirror (166 down to 0)
- `0x800a5158` — 32-bit, a pointer to the rescue craft's object. If it is non-zero, also
  read the 32-bit word at `that value + 0x30` (its scene node), and then the three
  32-bit words at `node + 0x14`, `+0x18`, `+0x1c` — the craft's position.

## Only if it is genuinely cheap for you

A coarse path sample. **Do not** attempt per-frame capture; it is not worth the latency.
Instead, if you can step or run briefly and re-read, give me the craft's position at a
handful of countdown values — say 150, 120, 100, 80, 50, 20, 0 — as
`countdown,x,y,z`. Six or seven rows is plenty. Skip this entirely if it means fighting
the emulator.

## Reporting

- the section dump, or the 321 halfwords
- the countdown value and the craft pointer/node/position from the paused frame
- anything that reads as zero or nonsense — say so plainly rather than working around it,
  because a null pointer tells me the object is populated at a different time than I think

Negative results are useful here. If no section has bit `0x01`, that is a real answer and
changes how the port implements object paths.
