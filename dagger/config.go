package main

const (
	MainPackagePath = "./cmd/gobot-simulate"
	BinaryName      = "gobot-simulate"
	WebSourceDir    = "web"
	WebOutputDir    = ".output/public"
	NginxStaticPath = "/usr/share/nginx/html"
	WebDistTarget   = "webui/dist"
	BinTarget       = "bin"
	NodeVersion     = "24"
	PackageManager  = "npm"
	PnpmVersion     = "10"
	NodeTestImage   = "node:24-alpine"
	PlaywrightImage = "mcr.microsoft.com/playwright:v1.65.0-jammy"
)

var DefaultLdflags = []string{"-s", "-w"}
