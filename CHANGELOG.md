# Changelog

Every change to tuilib's public API (`pkg/`) is recorded here, newest first.
Each release's section is also its GitHub release notes.

## Upgrading

To move an app from one version to another, read every section above the
version you are on up to the version you are going to, and work **oldest
first**:

1. `go get github.com/jsdrews/tuilib@<version>`
2. Apply each **Breaking** entry's migration. Each one gives the code before
   and after the change.
3. Run `go fix ./...` (Go 1.26+). A **Deprecated** entry marked `go:fix
   inline` is rewritten for you; the old name keeps working until a later
   release removes it under **Breaking**.
4. `go build ./... && go test ./...`

**Added** and **Changed** entries need no action. They say what is now
available and what behaves differently, so an app can adopt them.

## Writing entries

Add entries under `## Unreleased` in the same change that makes them. The
release job reads that section to pick the next version, publishes it as the
release notes, and moves it under the new version's heading. Never write a
version heading by hand.

| Section | For | Version bump |
|---|---|---|
| `### Breaking` | anything that makes a consumer change code: a removal, a rename, a changed signature, a behavior an app has to adjust to | minor before v1, major after |
| `### Deprecated` | an old name kept working beside its replacement | minor |
| `### Added` | new API | minor |
| `### Changed` | a behavior change that needs no code change | minor |
| `### Fixed` | a bug fix | patch, when it is the only section |

Each Breaking change gets a `####` heading of its own and a `**Migration:**`
block (`go run ./internal/changelog/cmd/changelog check` enforces both):

````markdown
### Breaking

#### `table.Options.Foo` is now `table.Options.Bar`

One or two sentences on what changed and why.

**Migration:**

```go
// before
opts.Foo = 3
// after
opts.Bar = 3
```
````

Other sections are bullets naming the symbol: ``- `palette.Palette.Success`:
a foreground for good outcomes.``

## Unreleased

## v0.44.0 - 2026-10-26

### Added

- `palette.Palette.Success`: a foreground for a good outcome (Running, Ready,
  Succeeded, Healthy), set in every built-in and named palette. Base16
  palettes take `base0B`. Left unset in a custom palette, `theme.Compose`
  falls back to `InfoBG`, so existing palettes need no change. Use
  `t.Success` instead of a literal green for status values that should
  follow the palette.
