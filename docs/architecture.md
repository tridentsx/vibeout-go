# Runtime architecture

Dependencies point inward toward runtime state and never back toward SDL:

```text
main
 ├─ assets ── psx
 ├─ physics ── game, assets geometry
 ├─ render ── game, assets, SDL
 ├─ controller ── SDL input
 ├─ audio/sfx
 └─ audio/music
```

- `internal/psx` is the binary compatibility layer. It parses individual
  retail formats and contains no game policy, filesystem search, SDL, or audio
  backend code.
- `internal/assets` owns filesystem paths and assembles related decoded files
  into resources such as a complete track or decoded sound clip.
- `internal/game` owns runtime entities and value types only (`Ship`, `Vector3`,
  `Angle`). It does not load, simulate, or render them.
- `internal/physics` owns deterministic simulation and collision systems. It
  mutates `game` state and consumes track geometry through `assets`.
- `internal/render` owns cameras, presentation transforms, SDL textures, and
  draw calls. It never reads files or advances physics.
- `internal/audio/sfx` owns short semantic event sounds and their player
  contract. `internal/audio/music` independently owns soundtrack discovery,
  streaming/playback control, and movie-audio tracks.
- `main` is the composition root: it creates concrete loaders/backends and
  connects the subsystems. It should contain orchestration, not subsystem
  implementation.

Future backends should implement interfaces in the subsystem they serve. In
particular, SDL audio implementations belong below `internal/audio`, not in
`game`, `assets`, or `main`.
