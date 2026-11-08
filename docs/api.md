# HTTP API reference (`/api/v1`)

All control-plane routes are under `/api/v1` and honour the optional bearer
token (`Authorization: Bearer <token>`), CORS allowlist and 64 KB body limit.
Errors use `{"error":{"code":"...","message":"...","boardId":"..."}}`.

## State and health

| Method | Path | Description |
|---|---|---|
| GET | `/health` | status, version, uptime |
| GET | `/snapshot` | full metadata + boards |
| GET | `/boards` | board list |
| GET | `/boards/:board` | one board |
| GET | `/events` | SSE stream (`snapshot`, state events, `trace`, `heartbeat`) |

## Pins, devices and values

| Method | Path | Description |
|---|---|---|
| POST | `/boards/:board/pins/:pin` | set digital/byte/analog value or mode |
| GET | `/boards/:board/devices` | device list |
| GET | `/boards/:board/devices/:device` | one device |
| PATCH | `/boards/:board/devices/:device` | inputs/values only; outputs are observe-first |
| POST | `/boards/:board/devices/:device/press` | press a button/captor (optional `holdMs`) |
| POST | `/boards/:board/devices/:device/release` | release a button/captor |
| POST | `/boards/:board/devices/:device/command` | `{action,value}` through the attached driver |
| POST | `/boards/:board/devices/:device/fault` | inject a device fault |
| DELETE | `/boards/:board/devices/:device/fault` | clear one or all device faults |
| GET | `/boards/:board/values/:name` | read an aREST value |
| PUT | `/boards/:board/values/:name` | `{value}` write |
| POST | `/boards/:board/functions/:name` | invoke/configure an aREST function |
| POST | `/boards/:board/fault` | board-wide fault |
| DELETE | `/boards/:board/fault` | clear board fault |

## Trace and timeline

| Method | Path | Description |
|---|---|---|
| GET | `/boards/:board/trace?since=&kinds=&target=` | bounded trace query |
| GET | `/boards/:board/waveform?pins=&from=&to=&maxPoints=` | downsampled step series |
| POST | `/boards/:board/steps` | `{name,status,detail}` step marker |

## Buses, chips and topology

| Method | Path | Description |
|---|---|---|
| GET | `/boards/:board/buses` | bus inventory |
| GET | `/boards/:board/chips` | chip inventory + descriptors |
| GET | `/chips/:chip` | descriptor, live values and display snapshot |
| PATCH | `/chips/:chip` | `{field,value}` simulated quantity |
| POST | `/chips/:chip/raw` | `{write,readLen}` raw transaction |
| GET | `/topology` | effective post-merge topology |
| GET | `/topology/source` | original source document |

## Scenarios

| Method | Path | Description |
|---|---|---|
| GET | `/scenarios` | rules |
| PATCH | `/scenarios/:id` | `{enabled}` |
| POST | `/scenarios/:id/trigger` | manually fire |

## Protocol emulation

- `ANY /arest/:board/*action` — aREST HTTP contract (see `docs/compatibility.md`).
- `ANY /v1/devices/:board/*action` — Particle cloud REST emulation.
- `Simulator.FirmataPipe` / `FirmataTCP` — Firmata over in-memory pipe or TCP.
- `Simulator.ARESTSerialResponse` — aREST serial line transport.

The aREST/Particle surfaces emulate physical device firmware and are not part
of the authenticated `/api/v1` control plane.
