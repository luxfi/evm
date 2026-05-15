# Versioning

## Lineage

**Active lineage: `v0.18.x`** (latest tag: `v0.18.10`).

Future bumps continue plain semver from `v0.18.10`:

- patch: `v0.18.11`, `v0.18.12`, ...
- minor: `v0.19.0`, `v0.20.0`, ...
- major: `v1.0.0` (when interface-breaking)

No pre-release suffixes (`-rc1`, `-beta`, `-pre`), no build metadata
(`+sha.abc123`), no prefixes beyond the leading `v`. Plain `vX.Y.Z`.

## Historical / outlier tags

`v1.99.18` is preserved as immutable git history but is **not** the line of
active development. It was an experimental fork-style tag (the `1.99.x` series
was an out-of-band scheme that did not follow the active semver lineage).

Consumers and downstream go modules should pin `v0.18.x`. Do not derive
future versions from `v1.99.x`.

## Rules

1. One lineage. `v0.18.x` is canonical.
2. Plain semver. `vMAJOR.MINOR.PATCH` only.
3. Patch bumps are additive (`+1`). Never skip ahead to a new major to
   sidestep work.
4. Tags are immutable. Outliers stay in history; we do not retag, retroactively
   delete, or rewrite them.
