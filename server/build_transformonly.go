package server

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/esm-dev/esm.sh/server/common"
	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/utils"
)

type TransformOptions struct {
	Filename        string          `json:"filename"`
	Lang            string          `json:"lang"`
	Code            string          `json:"code"`
	ImportMap       json.RawMessage `json:"importMap"`
	JsxImportSource string          `json:"jsxImportSource"`
	Target          string          `json:"target"`
	SourceMap       string          `json:"sourceMap"`
	Minify          bool            `json:"minify"`
}

type ResolvedTransformOptions struct {
	TransformOptions
	importMap     common.ImportMap
	globalVersion string
}

type TransformOutput struct {
	Code string `json:"code"`
	Map  string `json:"map"`
}

func transform(options *ResolvedTransformOptions) (out *TransformOutput, err error) {
	target := esbuild.ESNext
	if options.Target != "" {
		if t, ok := targets[options.Target]; ok {
			target = t
		} else {
			err = errors.New("invalid target")
			return
		}
	}

	loader := esbuild.LoaderJS
	sourceCode := options.Code
	jsxImportSource := options.JsxImportSource

	if options.Lang == "" && options.Filename != "" {
		filename, _ := utils.SplitByFirstByte(options.Filename, '?')
		_, basename := utils.SplitByLastByte(filename, '/')
		_, options.Lang = utils.SplitByLastByte(basename, '.')
	}
	switch options.Lang {
	case "js":
		loader = esbuild.LoaderJS
	case "jsx":
		loader = esbuild.LoaderJSX
	case "ts":
		loader = esbuild.LoaderTS
	case "tsx":
		loader = esbuild.LoaderTSX
	case "css":
		loader = esbuild.LoaderCSS
	default:
		err = errors.New("unsupported language:" + options.Lang)
		return
	}

	if jsxImportSource == "" && (loader == esbuild.LoaderJSX || loader == esbuild.LoaderTSX) {
		var ok bool
		for _, key := range []string{"@jsxRuntime", "@jsxImportSource", "preact", "react"} {
			jsxImportSource, ok = options.importMap.Resolve(key)
			if ok {
				break
			}
		}
		
		// 处理HTTP模块中的React导入
		if !ok && isHttpSepcifier(options.Filename) {
			reactImport := extractReactImportFromCode(options.Code)
			if reactImport != "" {
				jsxImportSource = reactImport
				if options.importMap.Imports == nil {
					options.importMap.Imports = make(map[string]string)
				}
				options.importMap.Imports["react/jsx-runtime"] = jsxImportSource + "/jsx-runtime"
			} else {
				jsxImportSource = "react"
			}
		} else if !ok {
			jsxImportSource = "react"
		}
	}

	sourceMap := esbuild.SourceMapNone
	if options.SourceMap == "external" {
		sourceMap = esbuild.SourceMapExternal
	} else if options.SourceMap == "inline" {
		sourceMap = esbuild.SourceMapInline
	}

	filename := options.Filename
	if filename == "" {
		filename = "source." + options.Lang
	}
	stdin := &esbuild.StdinOptions{
		Sourcefile: filename,
		Contents:   sourceCode,
		Loader:     loader,
	}
	opts := esbuild.BuildOptions{
		Stdin:             stdin,
		Platform:          esbuild.PlatformBrowser,
		Format:            esbuild.FormatESModule,
		Target:            target,
		JSX:               esbuild.JSXAutomatic,
		JSXImportSource:   strings.TrimSuffix(jsxImportSource, "/"),
		MinifyWhitespace:  options.Minify,
		MinifySyntax:      options.Minify,
		MinifyIdentifiers: options.Minify,
		Sourcemap:         sourceMap,
		Bundle:            true,
		Outdir:            "/esbuild",
		Write:             false,
		Plugins: []esbuild.Plugin{
			{
				Name: "resolver",
				Setup: func(build esbuild.PluginBuild) {
					build.OnResolve(esbuild.OnResolveOptions{Filter: ".*"}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
						path, _ := options.importMap.Resolve(args.Path)
						return esbuild.OnResolveResult{Path: path, External: true}, nil
					})
				},
			},
		},
	}
	ret := esbuild.Build(opts)
	if len(ret.Errors) > 0 {
		err = errors.New("failed to validate code: " + ret.Errors[0].Text)
		return
	}
	if len(ret.OutputFiles) == 0 {
		err = errors.New("failed to validate code: no output files")
		return
	}
	out = &TransformOutput{}
	for _, file := range ret.OutputFiles {
		if strings.HasSuffix(file.Path, ".js") || strings.HasSuffix(file.Path, ".css") {
			out.Code = string(file.Contents)
		} else if strings.HasSuffix(file.Path, ".map") {
			out.Map = string(file.Contents)
		}
	}
	return
}

// extractReactImportFromCode 从代码中提取React导入URL，用于HTTP模块处理
func extractReactImportFromCode(code string) string {
	// 首先尝试标准的import语法格式
	re := regexp.MustCompile(`from\s+['"]([^'"]*(?:[rR]eact)@[^'"]+)['"]`)
	matches := re.FindStringSubmatch(code)
	if len(matches) > 1 {
		return matches[1]
	}
	
	// 如果没有找到标准格式，尝试匹配任何包含react@的引号字符串
	re = regexp.MustCompile(`['"]([^'"]*(?:[rR]eact)@[^'"]+)['"]`)
	matches = re.FindStringSubmatch(code)
	if len(matches) > 1 {
		return matches[1]
	}
	
	return ""
}
