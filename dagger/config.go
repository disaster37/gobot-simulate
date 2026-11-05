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
	PlaywrightImage = "mcr.microsoft.com/playwright:v1.64.1-jammy"
)

var DefaultLdflags = []string{"-s", "-w"}
