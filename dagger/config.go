package main

const (
	MainPackagePath = "./cmd/gobot-simulate"
	BinaryName      = "gobot-simulate"
	WebSourceDir    = "web"
	WebOutputDir    = ".output/public"
	NginxStaticPath = "/usr/share/nginx/html"
	WebDistTarget   = "webui/dist"
	BinTarget       = "bin"
	NodeVersion     = "22"
	PackageManager  = "npm"
	PnpmVersion     = "10"
	NodeTestImage   = "node:22-alpine"
)

var DefaultLdflags = []string{"-s", "-w"}
