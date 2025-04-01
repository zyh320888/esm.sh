# esm.d8d.fun CLI

A _nobuild_ tool for modern web development.

## Installation

You can install `esm.d8d.fun` CLI from source code:

```bash
go install github.com/zyh320888/esm.sh
```

You can also install `esm.d8d.fun` CLI via `npm`:

```bash
npm install -g esm.d8d.fun
```

Or use `npx` without installation:

```bash
npx esm.d8d.fun [command]
```

### Usage

```
$ esm.d8d.fun --help
Usage: esm.d8d.fun [command] <options>

Commands:
  i, add [...pakcage]   Alias to 'esm.d8d.fun im add'.
  im, importmap         Manage "importmap" script.
  init                  Create a new nobuild web app with esm.d8d.fun CDN.
  serve                 Serve a nobuild web app with esm.d8d.fun CDN, HMR, transforming TS/Vue/Svelte on the fly.
  build                 Build a nobuild web app with esm.d8d.fun CDN.
```
