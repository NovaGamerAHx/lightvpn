//go:build !windows && !bindings

package main

// main_other.go — the Wails window is Windows-only (WebView2). On other systems
// the binary still builds and runs as a CLI over the exact same engine, which is
// how the backend is tested on Linux/macOS.
//
// Under `-tags bindings` a different main takes over (main_bindings.go) so that
// `wails build` can generate the JS bindings while cross-compiling from
// Linux/macOS, exactly like a stock Wails project.

import (
	"fmt"
	"os"
	"runtime"
)

func main() {
	if len(os.Args) > 1 && runCLI(os.Args[1:]) {
		return
	}
	fmt.Printf("%s %s (xray-core %s, %s/%s)\n\n", appName, appVersion, xrayVersionString(), runtime.GOOS, runtime.GOARCH)
	fmt.Println("The GUI is built for Windows 10/11 (WebView2). On " + runtime.GOOS + " use the CLI:")
	fmt.Println()
	fmt.Print(cliUsage)
	os.Exit(2)
}
