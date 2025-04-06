package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/esm-dev/esm.sh/cli"
)

const helpMessage = "\033[30mesm.sh - A nobuild tool for modern web development.\033[0m" + `

Usage: esm.sh [command] <options>

Commands:
  i, add [...pakcage]   Alias to 'esm.sh im add'.
  im, importmap         Manage "importmap" script.
  init                  Create a new nobuild web app with esm.sh CDN.
  serve                 Serve a nobuild web app with esm.sh CDN, HMR, transforming TS/Vue/Svelte on the fly.
  download              Download app and all dependencies to local directory.

Download Options:
  --out-dir <dir>       Specify output directory (default: "dist").
  --minify              Minify downloaded code.
  --api-url <url>       Use custom API base URL (default: "https://esm.sh").
  --deno-json <file>    Specify a deno.json file path to use as importmap source.
  --base-path <path>    Add a base path prefix to all generated URLs (useful when app is not served from root).
`

//go:embed cli/internal
//go:embed cli/demo
var fs embed.FS

func main() {
	if len(os.Args) < 2 {
		fmt.Print(helpMessage)
		return
	}
	switch command := os.Args[1]; command {
	case "i", "add":
		cli.ManageImportMap("add")
	case "im", "importmap":
		cli.ManageImportMap("")
	case "init":
		cli.Init(&fs)
	case "serve":
		cli.Serve(&fs)
	case "download":
		cli.DownloadDependencies(os.Args[2:])
	default:
		fmt.Print(helpMessage)
	}
}
