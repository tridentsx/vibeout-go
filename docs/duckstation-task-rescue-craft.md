# Task for the DuckStation system

You have MCP access to DuckStation running **WipeOut 2097 PAL (SLES_003.27)**. I need
you to find what moves a specific object in RAM, and to capture its motion.

## Background

At the start of every race a maintenance/rescue craft hovers above the player's pad,
releases the player's ship, and then flies away as the countdown finishes. I have
reverse-engineered everything about how it *looks* but nothing about how it *moves* —
static analysis of the executable cannot find the code that writes its position,
because it is reached through a scene-graph walk or a computed index rather than by a
direct reference.

## The addresses

PS1 RAM is 2 MB. Addresses below are KSEG0 (`0x800xxxxx`); DuckStation's memory tools
may expect physical addresses instead, in which case **drop the `0x8` and use
`0x000xxxxx`** — for example `0x800a5158` becomes `0x000a5158`. Everything is
**little-endian 32-bit**.

1. `0x800a5158` holds a **pointer** to the rescue craft's object. Read it as a 32-bit
   word. Call the value `OBJ`. It should look like a RAM address, roughly
   `0x800c0000`–`0x801f0000`. It is only valid once a race has loaded.

2. `OBJ + 0x30` holds a **pointer** to that object's scene node. Read it. Call the
   value `NODE`.

3. `NODE + 0x14`, `+0x18`, `+0x1c` are the node's translation **X, Y, Z**, each a
   signed 32-bit integer. `NODE + 0x40` is a 16-bit flag the engine sets to 1 when the
   translation changes.

There is also a useful clock: the player ship's countdown. `0x800949b4` is a 32-bit
mirror of it, counting **166 down to 0**, one per tick at 25 Hz. Phases: the intro
camera sweep runs 166→100, the ship is released at 100, red lights 83→42, amber 41→1,
green at 0. The craft departs after 0.

## What I need, in priority order

### 1. The function that writes the craft's position

Start a single race (Talon's Reach is fine, any team). During the intro camera sweep,
pause and read `OBJ` then `NODE` as above. Then set a **write watchpoint on
`NODE + 0x14`, 4 bytes**, and resume.

When it triggers, report **the program counter** — the address of the instruction that
performed the write — and ideally a short disassembly around it. That address is the
whole answer; everything else below is a fallback.

If a watchpoint is not available, try a write breakpoint on the same address, or
DuckStation's memory-access tracing if it has any.

### 2. The craft's actual flight path

Equally valuable and probably easier. Sample, once per frame or as often as you can
manage, from the moment the race loads until about ten seconds after the countdown
reaches zero:

- `0x800949b4` (the countdown)
- `NODE + 0x14`, `+0x18`, `+0x1c` (the craft's X, Y, Z)

Report it as CSV: `countdown,x,y,z`. Note that **negative Y is up** in this engine.

This alone lets me reproduce the motion even without finding the code, so please do it
even if step 1 succeeds.

### 3. Two things worth confirming while you are there

- Read the player's ship position for comparison. `0x80095858` holds a pointer to the
  ship array; each ship is `0xf0` bytes; the player is index 0; its position is at
  `+0x40`, `+0x44`, `+0x48` as signed 32-bit X, Y, Z. I expect the player to start
  about **300 units above** the pad and fall onto it as the countdown runs, and the
  craft to hover **above that** without descending with it. Confirming or contradicting
  that is useful either way.
- Watch whether `NODE + 0x40` is being set to 1 each time the position changes. If the
  position moves while that flag stays 0, the mover is writing the translation directly
  rather than through the engine's helper, which is itself a finding.

## Reporting back

Please give me:

- the watchpoint's program counter, if you got one
- the CSV of countdown against craft position
- the player ship position at a few countdown values
- anything that contradicts the expectations above — that is more useful than
  confirmation

If `OBJ` reads as zero or nonsense, say so rather than working around it: it means the
pointer is only populated at a different point than I think, which is itself worth
knowing.

---

# Addendum: two sharper questions

Since this task was written, the code that moves the craft has been found by static
analysis after all: a waypoint path state machine at `0x800676d4` and `0x80067198`.
That changes what is still worth measuring.

## A. Does anything set section flag bit 0x01 at runtime?

This is now the most valuable single answer.

`maybe_InitMovingObjectPath` (`0x80067864`) decides where a moving object's path begins
by walking the track's section list and stopping at the first section whose flag
halfword has **bit 0x01** set:

```c
entity[0] = trackSections;
do { entity[0] = entity[0]->Next; } while (!(entity[0]->0x96 & 1));
```

But **no section on TRACK01 has that bit set on disc.** Values there are 0, 0x20, 0x28
and 0x30 only. So either something sets it after the track loads, or that walk simply
runs to its section-count bound and the path starts wherever it happens to stop.

To find out: with a race loaded on Talon's Reach, read the section array and count how
many sections have bit 0x01 in the halfword at section `+0x96`.

- `0x8009553c` holds a pointer to the track data structure. Read it, then read the word
  at `+0x14` of that — call it `SECTIONS`.
- Sections are **156 bytes** each; TRACK01 has **321** of them.
- For each section *i*, read the 16-bit halfword at `SECTIONS + i*156 + 0x96`.

Report how many have bit 0x01 set, and their indices if there are few. If the answer is
zero, that settles it the other way, which is equally useful.

## B. The craft's path, as data

Still worth capturing, as originally described: sample `0x800949b4` (the countdown) and
the craft's node translation once per frame through the whole start sequence, and report
it as CSV. With the mechanism understood, a recorded path lets the phase durations and
waypoint spacing be checked against it rather than derived by reading every constant.

## What is no longer needed

The watchpoint on `NODE + 0x14`. It already did its job -- it pointed at the function --
and the write is now understood:

```asm
lw $v0, 120($s1)      ; prop pool + 0x78, slot 0x1e
lw $a0, 48($v0)       ; object + 0x30, its node
jal maybe_SetNodeTranslation
```
