package main

import (
	"context"

	"dagger/gobot-simulate/internal/dagger"
)

type GobotSimulate struct {
	Src *dagger.Directory
}

func New(
	// +defaultPath="."
	// +ignore=["bin", "coverage.out", "web/node_modules", "web/.nuxt", "web/.output"]
	src *dagger.Directory,
) *GobotSimulate {
	return &GobotSimulate{Src: src}
}

// Functions lists the available pipeline entrypoints.
func (m *GobotSimulate) Functions(_ context.Context) ([]string, error) {
	return []string{
		"ci",
		"lint",
		"test",
		"test-race",
		"vulncheck",
		"web-build",
		"web-test",
		"build",
		"dev",
	}, nil
}

// Lint runs golangci-lint via the remote golang Daggerverse module.
func (m *GobotSimulate) Lint(ctx context.Context) (string, error) {
	return dag.Golang(m.Src).Lint(ctx)
}

// Vulncheck runs the latest govulncheck inside the pinned golang module
// container. The remote module pins govulncheck v1.1.4, whose x/tools version
// panics while analysing Go 1.27 syntax.
func (m *GobotSimulate) Vulncheck(ctx context.Context) (string, error) {
	return dag.Golang(m.Src).
		Container().
		WithExec([]string{"go", "install", "golang.org/x/vuln/cmd/govulncheck@latest"}).
		WithExec([]string{"sh", "-c", "$(go env GOPATH)/bin/govulncheck ./..."}).
		Stdout(ctx)
}

// Test runs unit tests with coverage via the remote golang Daggerverse module.
func (m *GobotSimulate) Test(ctx context.Context) error {
	_, err := dag.Golang(m.Src).Test(dagger.GolangTestOpts{}).Sync(ctx)
	return err
}

// TestRace runs Go unit tests with the race detector enabled.
func (m *GobotSimulate) TestRace(ctx context.Context) (string, error) {
	return dag.Golang(m.Src).
		Container().
		WithEnvVariable("CGO_ENABLED", "1").
		WithExec([]string{"go", "test", "-race", "-count=1", "./..."}).
		Stdout(ctx)
}

// WebTest runs Vitest unit tests for the Nuxt 4 frontend inside a Node 22 container.
func (m *GobotSimulate) WebTest(ctx context.Context) (string, error) {
	webDir := m.Src.Directory(WebSourceDir)
	return dag.Container().
		From(NodeTestImage).
		WithDirectory("/src", webDir).
		WithWorkdir("/src").
		WithMountedCache("/root/.npm", dag.CacheVolume("npm-cache-node22")).
		WithExec([]string{"npm", "ci"}).
		WithExec([]string{"npm", "run", "test:unit"}).
		Stdout(ctx)
}

// WebBuild builds the static Nuxt 4 SPA and extracts the nginx static root.
func (m *GobotSimulate) WebBuild(ctx context.Context) (*dagger.Directory, error) {
	webDir := m.Src.Directory(WebSourceDir)
	ctr := dag.Web().BuildStatic(webDir, PackageManager, NodeVersion, WebOutputDir, "", PnpmVersion)
	dist := ctr.Directory(NginxStaticPath)
	if _, err := dist.File("index.html").Sync(ctx); err != nil {
		return nil, err
	}
	return dist, nil
}

// Build compiles the standalone simulator binary with a freshly built webui/dist.
func (m *GobotSimulate) Build(ctx context.Context) (*dagger.Directory, error) {
	webDist, err := m.WebBuild(ctx)
	if err != nil {
		return nil, err
	}
	return m.buildBinary(m.Src.WithDirectory(WebDistTarget, webDist)), nil
}

// +private
func (m *GobotSimulate) buildBinary(src *dagger.Directory) *dagger.Directory {
	buildDir := dag.Golang(src).Build(dagger.GolangBuildOpts{
		Main:    MainPackagePath,
		Out:     BinaryName,
		Ldflags: DefaultLdflags,
	})
	return dag.Directory().WithFile(BinaryName, buildDir.File(BinaryName))
}

// Ci executes the full pipeline in fail-fast order.
func (m *GobotSimulate) Ci(ctx context.Context) (*dagger.Directory, error) {
	webDist, err := m.WebBuild(ctx)
	if err != nil {
		return nil, err
	}
	src := m.Src.WithDirectory(WebDistTarget, webDist)

	if _, err := dag.Golang(src).Lint(ctx); err != nil {
		return nil, err
	}
	if _, err := m.Vulncheck(ctx); err != nil {
		return nil, err
	}
	if _, err := dag.Golang(src).Test(dagger.GolangTestOpts{}).Sync(ctx); err != nil {
		return nil, err
	}
	if _, err := dag.Golang(src).
		Container().
		WithEnvVariable("CGO_ENABLED", "1").
		WithExec([]string{"go", "test", "-race", "-count=1", "./..."}).
		Stdout(ctx); err != nil {
		return nil, err
	}
	if _, err := m.WebTest(ctx); err != nil {
		return nil, err
	}
	return dag.Directory().
		WithDirectory(WebDistTarget, webDist).
		WithDirectory(BinTarget, m.buildBinary(src)), nil
}

// Dev runs the simulator and embedded UI as a Dagger service on port 8080.
func (m *GobotSimulate) Dev(ctx context.Context) (*dagger.Service, error) {
	webDist, err := m.WebBuild(ctx)
	if err != nil {
		return nil, err
	}
	src := m.Src.WithDirectory(WebDistTarget, webDist)
	return dag.Golang(src).
		Container().
		WithExposedPort(8080).
		AsService(dagger.ContainerAsServiceOpts{
			Args: []string{"go", "run", MainPackagePath, "--addr", "0.0.0.0", "--port", "8080"},
		}), nil
}
