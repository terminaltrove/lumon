# Layout

```
cmd/lumon/            flags and wiring; LAN games
internal/
  lumon/              the board, one player's view of it, and the diversions
  canvas/             a flat cell buffer, and braille for 2x4 sub-cell drawing
  physics/            Verlet, constraints, springs, a fixed-timestep clock
  glyph/              a digit as strokes, rasterised and quantised to characters
  theme/              a palette of roles resolved into drawing attributes
  noise/              the Perlin field that decides which numbers are corrupt
  agent/              telling a game what each coding agent is doing
  ease/               the two easing helpers that have no other home
plugins/claude-code/  the Claude Code plugin
plugins/pi/           the Pi extension
plugins/opencode/     the OpenCode plugin
scripts/              build, pin the plugins
licenses/             what was ported from where
```

The ordering is by dependency. `noise`, `ease`, `canvas` and `physics` know
nothing about anything else; `glyph` and `theme` know only about drawing; `lumon`
knows about all of them. The test of whether something belongs outside `lumon` is
whether it can be described without mentioning a board or a screen.

Everything is under `internal/` because only `cmd/lumon` imports it, so a
signature can change without breaking anyone else.

`internal/lumon/doc.go` covers why `Model` embeds `*Room`, why that pair is not
split further, and why the diversions are still in that package.

## Building

```bash
./scripts/build.sh    # every target, into dist/, with checksums
( cd dist && shasum -a 256 -c checksums.sha256sum )
```

`linux/386` stays in the target list. `addressBase` is `0xFFC00000`, which does
not fit in a 32-bit `int`, and an untyped constant takes its type from the `int`
it is combined with. `go vet` type-checks per architecture, so building this
target is what catches that.

## Testing

```bash
go test ./... -race -count=1              # the whole suite
go test ./internal/lumon -run TestName    # one
```

Tests are behaviour-named prose and assert on outcomes, not internals.
`internal/noise/perlin_test.go` pins the noise against golden values that keep
the board identical to the C++ reference it is ported from. Do not change them.

`TestAFloorOfHundredsKeepsUp` measures how many refiners this machine can serve,
so it needs a fast one, and fails on a slow laptop without anything being wrong.
