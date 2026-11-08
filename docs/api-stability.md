# API stability contract

`/api/v1` and the exported Go package `github.com/disaster37/gobot-simulate`
are frozen at the first semver tag.

## Policy after the first tag

- Changes are **additive only**: new endpoints, fields, device kinds, chips,
  options and helper functions may be added in a minor release.
- Existing request/response shapes and exported Go identifiers keep their
  meaning for the lifetime of `v1`.
- Deprecations are announced in `CHANGELOG.md`, keep working for at least one
  minor release, and are removed only in a major release.
- Incompatible changes go to `/api/v2` (and, if unavoidable, a new Go module
  major version). `v1` continues to be served during the documented migration
  window.

## Versioning

- Semantic version tags (`vMAJOR.MINOR.PATCH`) drive Go module consumers.
- The binary version is injected at build time:
  `-X github.com/disaster37/gobot-simulate.Version=vX.Y.Z`. The default is
  `0.1.0-dev` for untagged builds.
- The version is exposed by `GET /api/v1/health`, `GET /api/v1/snapshot`
  metadata and the UI footer.
- The npm layer `@disaster37/gobot-simulate-ui` follows the same version.

## Schema references

- Topology and scenario schema: [`docs/topology.md`](topology.md)
- Generated driver support matrix: [`docs/support-matrix.md`](support-matrix.md)
- gobot compatibility: [`docs/compatibility.md`](compatibility.md)
