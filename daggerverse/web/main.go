package main

import "dagger/web/internal/dagger"

type Web struct{}

func New() *Web { return &Web{} }

func (m *Web) BuildStatic(src *dagger.Directory, packageManager string, nodeVersion string, outputDir string, nginxConfigPath string, pnpmVersion string) *dagger.Container {
	built := dag.Container().
		From("node:"+nodeVersion+"-alpine").
		WithDirectory("/app", src).
		WithWorkdir("/app").
		WithMountedCache("/root/.npm", dag.CacheVolume("npm-cache-node22")).
		WithExec([]string{"npm", "ci"}).
		WithExec([]string{"npm", "run", "build"})
	return dag.Container().
		From("nginx:alpine").
		WithDirectory("/usr/share/nginx/html", built.Directory("/app/"+outputDir)).
		WithExec([]string{"rm", "-f", "/usr/share/nginx/html/50x.html"})
}
