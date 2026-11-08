# Compatibility matrix

| gobot-simulate | gobot (Go module) | gobot-arest (test-only) | Go | Node | Nuxt / Nuxt UI |
|---|---|---|---|---|---|
| `v0.1.x` | `gobot.io/x/gobot/v2 v2.6.0` | `github.com/disaster37/gobot-arest/v2 v2.0.0` | 1.27+ | 24 | Nuxt 4 / Nuxt UI 4 |

The simulator pins the gobot module in `go.mod`. Board profiles are generated
from that exact module source; upgrading gobot requires regenerating
`boardprofile/` with `make generate` and committing the diff.

`gobot-arest/v2` is used only by conformance tests and is never imported by the
production simulator.
