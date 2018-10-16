/*
Sniperkit-Bot
- Status: analyzed
*/

// +build dev

package config

const (
	DEV = true

	ErrorKind      = "ErrorDev"
	CompileKind    = "CompileDev"
	PackageKind    = "PackageDev"
	DeployKind     = "DeployDev"
	ShareKind      = "ShareDev"
	HintsKind      = "HintsDev"
	WasmDeployKind = "WasmDeployDev"
)

var Bucket = map[string]string{
	Src:   "dev-src.jsgo.io",
	Pkg:   "dev-pkg.jsgo.io",
	Index: "dev-index.jsgo.io",
	Git:   "dev-git.jsgo.io",
}
