# gobot-simulate

An in-memory, thread-safe [Gobot](https://gobot.io) adaptor plus a Hertz REST/SSE API and an embedded Nuxt UI for inspecting and driving simulated boards.

It is a drop-in replacement for `github.com/disaster37/gobot-fat/mock.MockPlateform` and satisfies the `DFPAdaptor`, `TFPAdaptor`, and `TankAdaptor` interfaces used by `gobot-fat`, without importing `gobot-arest` or requiring physical hardware.

## Quick start

```bash
# Standalone simulator + demo DFP/TFP/tank boards and embedded UI
go run ./cmd/gobot-simulate --port 8080
# open http://127.0.0.1:8080

# Full Dagger pipeline
dagger develop
dagger call -m . ci export --path .
make verify-web-dist
```

Native development:

```bash
make dev-native                 # Go API + embedded UI on :8080
npm --prefix web run dev        # Nuxt HMR on :3000, proxying /api/v1 to :8080
```

## Architecture

| Path | Purpose |
|---|---|
| `simulate.go` | `Simulator` lifecycle, options, inspection/mutation API, `WaitEvent` |
| `adaptor.go` | `*Adaptor` / `MockPlateform`, Gobot + aREST + legacy methods |
| `discovery.go` | Reflection-free `AutoDiscover` using only exported driver methods |
| `config.go` | Configuration defaults, env/flag loading, validation |
| `internal/state` | Boards, pins, devices, values, functions, faults, Hub, ring buffer |
| `server/` | Hertz REST handlers, SSE stream, middleware, embedded SPA serving |
| `webui/` | `//go:embed all:dist` bundle accessor |
| `web/` | Nuxt 4 + Nuxt UI 4 + Tailwind 4 source and Vitest tests |
| `cmd/gobot-simulate` | Standalone demo binary |
| `examples/gobot-fat-demo` | Runnable `gobot-fat`-shaped DFP/TFP/tank example |
| `dagger/` | Thin local Dagger module (`ci`, `lint`, `test`, `test-race`, `vulncheck`, `web-build`, `web-test`, `build`, `dev`) |
| `daggerverse/web` | Vendored node/Nuxt static builder used by the Dagger module |

### Concurrency model

- `Store.mu` (RWMutex) protects the board/alias maps.
- Each `Board` has its own RWMutex protecting pins, devices, values, functions, faults, and test hooks.
- `Hub` and `RingBuffer` use their own locks; `Hub.Unsubscribe` never closes channels.
- Board locks are released before publishing events or invoking user `TestAdaptor*` hooks, so hooks may safely call back into the adaptor.
- All inspection APIs deep-copy maps and slices before returning.
- The event sequence number and ring insertion are serialized so replay order matches `seq`.

## Drop-in `gobot-fat` usage

### Option A — keep the existing mock import path

```go
package mock

import simulate "github.com/disaster37/gobot-simulate"

type MockPlateform = simulate.MockPlateform

var (
	NewMockPlateform = simulate.NewMockPlateform
	WaitEvent        = simulate.WaitEvent
)
```

Every legacy method is preserved: `SetInvertInitialPinState`, `SetInputPullup`, `Get/SetDigitalPinState`, `Get/SetValueReadState`, `Get/SetCallFunctionState`, `SetError`, `TestReconnect`, `TestAdaptor*`, and `Reconnect`. `ValuesRead()` always includes `"test": 99` merged with stored values.

### Option B — shared simulator and interactive UI

```go
sim := simulate.New(
    simulate.WithPort(8080),
    simulate.WithInputPullup("11", "13", "15"),
)

dfp := sim.Adaptor("dfp",
    simulate.WithAdaptorName("fat"),
    simulate.WithPlatform("raspberry-pi"),
    simulate.WithInvertedInitialPins("10", "12"),
    simulate.WithInputPullup("11", "13", "15", "10", "12", "19"),
)

_ = sim.AutoDiscover(dfpRobot,
    simulate.DeviceSpec{ID: "relay_drum", Label: "Drum Motor Relay", Kind: simulate.DeviceKindRelay, Pin: "3"},
    simulate.DeviceSpec{ID: "button_start", Label: "Start DFP", Kind: simulate.DeviceKindButton, Pin: "11", DefaultState: simulate.IntPtr(1)},
    simulate.DeviceSpec{ID: "captor_security_upper", Label: "Security Upper", Kind: simulate.DeviceKindCaptor, Pin: "10", DefaultState: simulate.IntPtr(0)},
)

if err := sim.Start(ctx); err != nil { /* handle */ }
defer sim.Stop(context.Background()) // registered before boardU.Stops so boards stop first
```

Scoped Viper keys verified against `gobot-fat`:

| Board | Prefix | Keys | Notes |
|---|---|---|---|
| DFP | `dfp.` | `name`, `fake-board`, `button_polling`, `pin.relay.drum`, `pin.relay.pomp`, `pin.led.green`, `pin.led.red`, `pin.button.emergency_stop`, `pin.button.start`, `pin.button.stop`, `pin.button.wash`, `pin.button.force_drum`, `pin.button.force_pump`, `pin.captor.security_upper`, `pin.captor.security_under`, `pin.captor.water_upper`, `pin.captor.water_under` | Current code uses these keys; the sample YAML still contains legacy names. |
| TFP | `tfp.` | `name`, `fake-board`, `url`, `pin.relay.pond_pomp` (inverted), `pin.relay.waterfall_pomp`, `pin.relay.uvc1`/`uvc2`/`filter_bubble`/`pond_bubble` (inverted) | Extra values `isRebooted` (false), function `acknoledgeRebooted`. |
| Tank | `tank_pond.` / `tank_garden.` | `name`, `fake-board`, `url` | Extra values `isRebooted`, `distance`; function `acknoledgeRebooted`. |

Button semantics follow Gobot exactly: a pull-up button with `DefaultState=1` is idle at `1`; `PressButton` writes `1-defaultState`, `ReleaseButton` restores `defaultState`. Inverted captors (`DefaultState=0`) behave symmetrically.

## Configuration

Precedence: explicit option/flag > environment variable > default.

| Env | Flag | Default | Notes |
|---|---|---|---|
| `SIMULATE_ADDR` | `--addr` | `127.0.0.1` | Listen address |
| `SIMULATE_PORT` | `--port` | `8080` | `0` selects an ephemeral port |
| `SIMULATE_UI` | `--ui` | `true` | Serve embedded Nuxt SPA |
| `SIMULATE_CORS` | `--cors` | `http://localhost:3000,http://127.0.0.1:3000` | Comma-separated allowlist |
| `SIMULATE_MAX_CLIENTS` | `--max-clients` | `64` | Maximum concurrent SSE clients |
| `SIMULATE_EVENT_BUFFER` | `--event-buffer` | `1024` | SSE replay ring buffer |
| `SIMULATE_LATENCY` | `--latency` | `0s` | Simulated I/O lag |
| `SIMULATE_AUTH_TOKEN` | `--auth-token` | empty | Optional constant-time bearer token |

## REST API

Base path `/api/v1`. Errors use:

```json
{"error":{"code":"DEVICE_NOT_FOUND","message":"simulate: device not found: relay_unknown","boardId":"dfp"}}
```

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Liveness, version, uptime |
| `GET` | `/snapshot` | Full simulator snapshot |
| `GET` | `/boards` | List boards |
| `GET` | `/boards/:board` | One board |
| `GET` | `/boards/:board/devices` | Devices on a board |
| `GET` | `/boards/:board/devices/:device` | One device |
| `PATCH` | `/boards/:board/devices/:device` | Set `active`, `value`, `byteValue`, or `angle` |
| `POST` | `/boards/:board/devices/:device/press` | Press; optional `{"holdMs":250}` pulse |
| `POST` | `/boards/:board/devices/:device/release` | Release |
| `POST` | `/boards/:board/pins/:pin` | Set `digitalValue`, `byteValue`, `analogValue`, or `mode` |
| `GET`/`PUT` | `/boards/:board/values/:name` | Read/write aREST values (`distance`, `isRebooted`, …) |
| `POST` | `/boards/:board/functions/:name` | Invoke function and/or set `returnCode` |
| `POST`/`DELETE` | `/boards/:board/fault` | Inject/clear a board fault |
| `GET` | `/events` | SSE stream |

Board/device/pin/value/function identifiers are restricted to `^[a-zA-Z0-9_.:-]{1,64}$`. Request bodies are capped at 64 KB.

### SSE contract

- Headers: `text/event-stream; charset=utf-8`, `Cache-Control: no-cache, no-transform`, `Connection: keep-alive`, `X-Accel-Buffering: no`.
- Initial frame: `retry: 2000`.
- Event frames: `id: <seq>\nevent: <type>\ndata: <compact JSON>\n\n`.
- Reconnect with `Last-Event-ID` (header) or `?lastEventId=`; events are replayed from the ring buffer, otherwise a fresh `event: snapshot` frame is sent.
- Heartbeat every 15s: `event: heartbeat\ndata: {"timestamp":"..."}`.
- At most `SIMULATE_MAX_CLIENTS` streams; excess requests get `429 TOO_MANY_CLIENTS`.
- With an auth token, `/events` accepts `?token=<token>` as well as `Authorization: Bearer`.

## Dagger pipeline

Root `dagger.json` pins engine `v0.21.8` and the `golang` Daggerverse module. The Nuxt static builder lives in `daggerverse/web` because the upstream `node@v0.6.0` module exports a root object named `Node`, which collides with the core Dagger `Node` interface in engine v0.21.8 and prevents SDK generation. The vendored module keeps the same positional `BuildStatic(src, packageManager, nodeVersion, outputDir, nginxConfigPath, pnpmVersion)` contract; the local module calls it as `dag.Web().BuildStatic(...)`.

```bash
dagger develop
dagger call -m . functions       # lists build/ci/dev/lint/src/test/test-race/vulncheck/web-build/web-test
dagger call -m . lint
dagger call -m . test-race
dagger call -m . web-build export --path ./webui/dist
dagger call -m . build export --path ./bin
dagger call -m . ci export --path .
```

`make ci` runs the Dagger CI pipeline, checks `webui/dist/index.html`, then `make verify-web-dist` (`git status --porcelain -- webui/dist`). `webui/dist` and generated Dagger bindings are committed.

## Security

- No `unsafe`; no reflection on unexported fields.
- Static assets are served from the embedded FS with path-traversal rejection; `/api/*` never falls back to `index.html`.
- Optional bearer auth is compared with `crypto/subtle`; tokens are never logged.
- CORS is allowlist-based.
- Security headers: `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, and a self-hosted CSP.
- Panic recovery returns the standard JSON error envelope.
- No outbound network calls from the simulator itself (SSRF-free).

## Testing

```bash
go test -count=1 ./...
go test -race -count=1 ./...
cd web && npm ci && npm run test:unit && npm run build
```

Coverage includes legacy `MockPlateform` interface assertions, pull-up/inverted semantics, fault propagation, re-entrant test hooks, concurrent store access, Hertz REST endpoints, SSE replay/`Last-Event-ID`, client caps, CORS/auth/body limits, static SPA fallback, and Vitest composable reducer/optimistic-rollback tests.
