# Changelog

All notable changes to gobot-simulate are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
semantic versioning.

## [Unreleased]

### Added

- Wiring model with drive polarity, NO/NC contact, mechanism, pull direction,
  derived `energised`/`loadPowered` state and `WIRING_MISMATCH` warnings.
- Device taxonomy, UI groups, device-level hardware faults and driver-aware
  commands through attached gobot drivers.
- Bounded I/O trace ring, waveform downsampling, SSE `event: trace` and
  business-level step annotations.
- Declarative topology loader (`SIMULATE_TOPOLOGY`, `--topology`) with
  path-qualified validation and effective-concept endpoints.
- Scenario engine with state/trace/timer/board triggers, actions, delays,
  ramps, fake-clock test mode, manual override and loop guard.
- aREST HTTP, aREST serial and Particle cloud emulation with driver-in-the-loop
  conformance tests.
- Reusable Nuxt UI layer (`@disaster37/gobot-simulate-ui`) plus standalone app,
  timeline, topology, scenario and bus chip views.
- Code-generated board profiles, generic register-map fallback and a generated
  driver support matrix.
- Wired GPIO/aio drivers, 1-Wire DS18B20, I2C/SPI chip breadth and BLE generic /
  microbit drivers.

## [0.1.0] - 2026-11-08

- Initial simulation platform foundation: adaptor, state store, REST/SSE API,
  embedded Nuxt UI and Dagger CI.
