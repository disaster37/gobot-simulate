# Vendored Dagger module: `web`

This directory is a **vendored, renamed** copy of the remote Daggerverse
module `gitlab.com/build-core/daggerverse/node@v0.6.0` (pinned commit
`40f0992e623b325f9fe3717ba01e144535f539e2`).

## Why it is vendored

The upstream module exposes a root object named `Node`. The Dagger engine
(v0.21.8) already ships a core `Node` GraphQL type, so the generated Go SDK
binding (`dagger/internal/dagger/node.gen.go`) would declare a second `Node`
type in the same package and collide with the core one, breaking `dagger
develop` / SDK generation. Renaming the *dependency* only changes the accessor
(`dag.Web()` vs `dag.Node()`); it does not change the module's own type name,
so the collision cannot be avoided with a dependency alias alone. Vendoring the
module under the renamed type `Web` sidesteps the collision.

See `README.md` ("Architecture" section) for the same rationale.

## What is mirrored

- `New()` constructor returning `*Web`.
- `BuildStatic(src, packageManager, nodeVersion, outputDir, nginxConfigPath,
  pnpmVersion) *Container` with the same positional signature as upstream.
- The two-stage build: `npm ci` + `npm run build` in a `node:<version>-alpine`
  container, then copy `outputDir` into `nginx:alpine` at
  `/usr/share/nginx/html`.

## What is intentionally dropped (do NOT rely on)

- **pnpm / bun / yarn package managers** — only `npm ci` is implemented
  (`packageManager` is accepted for signature compatibility but ignored).
- **Custom `nginxConfigPath`** — accepted for signature compatibility but
  ignored; no custom nginx config is mounted.
- **Server / dev mode and any nginx runtime tuning** — this is a static-build
  only wrapper.

## Sync policy

Any upstream change to `gitlab.com/build-core/daggerverse/node@v0.6.0` must be
**manually ported** to this file. There is no automated sync. When porting,
preserve the `Web` type name (do not rename back to `Node`) and keep the
positional `BuildStatic` signature so `dagger/internal/dagger/web.gen.go`
continues to generate correctly.

After any change, run `dagger develop` and re-export/verify the bundle:

```bash
dagger develop
dagger call -m . web-build export --path ./webui/dist
make verify-web-dist
```