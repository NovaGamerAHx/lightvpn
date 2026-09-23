//go:build !windows && bindings

package main

// main_bindings.go — `wails build` generates the frontend bindings by compiling
// the project a second time with `-tags bindings` and *running* that binary on
// the build machine; under that tag wails.Run() does not open a window, it only
// writes frontend/wailsjs/. Without this file the temporary tool would fall
// through to the CLI main and fail, which is why cross-building from Linux would
// otherwise need `-skipbindings`.

import (
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
)

func main() {
	app, cleanup, err := NewApp("")
	if err != nil {
		// Bindings only describe method signatures, so a broken config.json is
		// not a reason to fail the build.
		if app == nil {
			os.Exit(1)
		}
	}
	if cleanup != nil {
		defer cleanup()
	}

	// No AssetServer: under the bindings tag wails.Run only reflects over Bind.
	if err := wails.Run(&options.App{
		Title: appName,
		Bind:  []any{app},
	}); err != nil {
		os.Exit(1)
	}
}
