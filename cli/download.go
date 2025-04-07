package cli

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "regexp"
    "strings"
    "sync"
)

type DependencyInfo struct {
    Specifier string   `json:"specifier"`
    Dependencies []string `json:"dependencies"`
    Files map[string]string `json:"files"`
}

// 共享的模块映射，由下载过程填充
var globalModuleMap map[string]string
// 跟踪已经下载过的模块，避免重复下载
var downloadedModules map[string]bool
// 保护downloadedModules的互斥锁
var downloadedModulesMutex sync.Mutex
// 是否压缩代码
var minify bool
// API 基础 URL
var apiBaseURL string
// deno.json文件路径
var denoJsonPath string
// 基础路径，用于生成URL时添加前缀
var basePath string

// 获取API域名部分，用于路径处理
func getAPIDomain() string {
    return strings.TrimPrefix(strings.TrimPrefix(apiBaseURL, "https://"), "http://")
}

func DownloadDependencies(args []string) error {
    fmt.Println("开始执行下载命令...")
    
    // 初始化全局模块映射
    globalModuleMap = make(map[string]string)
    // 初始化已下载模块集合
    downloadedModules = make(map[string]bool)
    
    if len(args) < 1 {
        return fmt.Errorf("请指定入口文件或目录")
    }

    entryPath := args[0]
    outDir := "dist"
    minify = false
    // 默认使用 esm.sh 作为 API 基础 URL
    apiBaseURL = "https://esm.d8d.fun"
    // 默认deno.json路径为空
    denoJsonPath = ""
    // 默认basePath为空
    basePath = ""
    
    fmt.Printf("入口路径: %s\n", entryPath)
    
    // 从参数中获取输出目录和压缩选项
    for i := 1; i < len(args); i++ {
        if args[i] == "--out-dir" && i+1 < len(args) {
            outDir = args[i+1]
            fmt.Printf("输出目录: %s\n", outDir)
            i++
        } else if args[i] == "--minify" {
            minify = true
            fmt.Println("启用代码压缩")
        } else if args[i] == "--api-url" && i+1 < len(args) {
            apiBaseURL = args[i+1]
            fmt.Printf("使用API基础URL: %s\n", apiBaseURL)
            i++
        } else if args[i] == "--deno-json" && i+1 < len(args) {
            denoJsonPath = args[i+1]
            fmt.Printf("使用deno.json路径: %s\n", denoJsonPath)
            i++
        } else if args[i] == "--base-path" && i+1 < len(args) {
            basePath = args[i+1]
            // 确保basePath以/开头但不以/结尾
            if !strings.HasPrefix(basePath, "/") {
                basePath = "/" + basePath
            }
            if strings.HasSuffix(basePath, "/") {
                basePath = basePath[:len(basePath)-1]
            }
            fmt.Printf("使用基础路径: %s\n", basePath)
            i++
        }
    }

    // 检查入口是文件还是目录
    fileInfo, err := os.Stat(entryPath)
    if err != nil {
        fmt.Printf("获取入口信息失败: %v\n", err)
        return fmt.Errorf("获取入口信息失败: %v", err)
    }

    // 判断入口文件类型
    var actualEntryPath string
    var indexHtmlPath string
    if fileInfo.IsDir() {
        // 如果是目录，尝试找到 index.html
        fmt.Printf("%s 是目录，查找 index.html...\n", entryPath)
        indexHtmlPath = filepath.Join(entryPath, "index.html")
        if _, err := os.Stat(indexHtmlPath); err != nil {
            fmt.Printf("在目录 %s 中未找到 index.html: %v\n", entryPath, err)
            return fmt.Errorf("在目录 %s 中未找到 index.html: %v", entryPath, err)
        }
        fmt.Printf("找到入口文件: %s\n", indexHtmlPath)
        actualEntryPath = indexHtmlPath
    } else {
        // 直接使用文件
        actualEntryPath = entryPath
    }
    
    // 判断入口文件扩展名
    fileExt := filepath.Ext(actualEntryPath)
    fmt.Printf("入口文件扩展名: %s\n", fileExt)
    
    // 检查是否为前端源文件
    isFrontendSource := fileExt == ".tsx" || fileExt == ".ts" || fileExt == ".jsx" || fileExt == ".js"
    
    // 前端源文件需要指定deno.json
    if isFrontendSource && denoJsonPath == "" {
        fmt.Printf("入口文件是前端源文件 (%s)，需要同时指定 deno.json 文件\n", fileExt)
        return fmt.Errorf("入口文件是前端源文件 (%s)，需要同时使用 --deno-json 指定 deno.json 文件", fileExt)
    }
    
    var importMapData struct {
        Imports map[string]string `json:"imports"`
    }
    var entryContent []byte
    
    // 如果指定了deno.json文件路径，从deno.json读取importmap
    if denoJsonPath != "" {
        fmt.Printf("使用指定的deno.json文件: %s\n", denoJsonPath)
        
        // 读取deno.json文件
        denoJsonContent, err := os.ReadFile(denoJsonPath)
        if err != nil {
            fmt.Printf("读取deno.json文件失败: %v\n", err)
            return fmt.Errorf("读取deno.json文件失败: %v", err)
        }
        
        // 解析deno.json内容
        if err := json.Unmarshal(denoJsonContent, &importMapData); err != nil {
            fmt.Printf("解析deno.json内容失败: %v\n", err)
            return fmt.Errorf("解析deno.json内容失败: %v", err)
        }
        
        if importMapData.Imports == nil {
            fmt.Println("deno.json不包含有效的imports字段")
            return fmt.Errorf("deno.json不包含有效的imports字段")
        }
        
        fmt.Printf("从deno.json解析到的importmap: %v\n", importMapData.Imports)
        
        // 自动添加常用的React相关子模块
        addReactJsxRuntime(&importMapData)
    } else {
        // 从HTML中解析importmap
        // 如果是HTML文件，从中解析importmap
        fmt.Printf("入口文件是HTML文件，从中解析importmap\n")
        
        // 读取入口文件
        fmt.Printf("正在读取入口文件: %s\n", actualEntryPath)
        entryContent, err = os.ReadFile(actualEntryPath)
        if err != nil {
            fmt.Printf("读取入口文件失败: %v\n", err)
            return fmt.Errorf("读取入口文件失败: %v", err)
        }
        fmt.Println("入口文件读取成功")
        
        // 解析 importmap
        fmt.Println("正在解析 importmap...")
        
        // 使用正则表达式从 HTML 中提取 importmap
        importMapRegex := regexp.MustCompile(`<script\s+type="importmap"\s*>([\s\S]*?)<\/script>`)
        matches := importMapRegex.FindSubmatch(entryContent)
        
        if len(matches) < 2 {
            fmt.Println("未在入口文件中找到 importmap")
            return fmt.Errorf("未在入口文件中找到 importmap")
        }
        
        importMapJson := matches[1]
        fmt.Printf("找到 importmap: %s\n", string(importMapJson))
        
        if err := json.Unmarshal(importMapJson, &importMapData); err != nil {
            fmt.Printf("解析 importmap 失败: %v\n", err)
            return fmt.Errorf("解析 importmap 失败: %v", err)
        }
        
        if importMapData.Imports == nil {
            fmt.Println("importmap 不包含有效的 imports 字段")
            return fmt.Errorf("importmap 不包含有效的 imports 字段")
        }
        
        fmt.Printf("解析到的 importmap: %v\n", importMapData.Imports)
        
        // 自动添加常用的React相关子模块
        addReactJsxRuntime(&importMapData)
    }

    // 3. 创建输出目录
    fmt.Printf("正在创建输出目录: %s\n", outDir)
    if err := os.MkdirAll(outDir, 0755); err != nil {
        fmt.Printf("创建输出目录失败: %v\n", err)
        return fmt.Errorf("创建输出目录失败: %v", err)
    }
    
    // 从API URL中提取域名部分作为目录名
    apiDomain := getAPIDomain()
    
    // 创建目录
    esmDir := filepath.Join(outDir, apiDomain)
    if err := os.MkdirAll(esmDir, 0755); err != nil {
        fmt.Printf("创建 %s 目录失败: %v\n", apiDomain, err)
        return fmt.Errorf("创建 %s 目录失败: %v", apiDomain, err)
    }

    // 4. 使用并发下载所有依赖
    fmt.Printf("开始下载依赖，共 %d 个\n", len(importMapData.Imports))
    var wg sync.WaitGroup
    errChan := make(chan error, len(importMapData.Imports))
    semaphore := make(chan struct{}, 5) // 限制并发数
    
    // 保存模块URL和本地路径的映射
    moduleMap := make(map[string]string)

    // 下载所有依赖
    for spec, url := range importMapData.Imports {
        fmt.Printf("准备下载依赖: %s -> %s\n", spec, url)
        wg.Add(1)
        go func(spec, url string) {
            defer wg.Done()
            semaphore <- struct{}{}
            defer func() { <-semaphore }()

            fmt.Printf("开始下载: %s\n", spec)
            
            // 解析URL中的模块路径
            moduleRegex := regexp.MustCompile(`https://.*?/(.+)`)
            matches := moduleRegex.FindStringSubmatch(url)
            
            var modulePath string
            if len(matches) > 1 {
                modulePath = matches[1]
                // 处理URL中的查询参数
                if strings.Contains(modulePath, "?") {
                    modulePath = strings.Split(modulePath, "?")[0]
                }
            } else {
                modulePath = spec
                // 处理spec中可能的查询参数
                if strings.Contains(modulePath, "?") {
                    modulePath = strings.Split(modulePath, "?")[0]
                }
            }
            
            fmt.Printf("从URL中提取的模块路径: %s\n", modulePath)
            
            // 使用传入的输出目录和API域名
            esmDir := filepath.Join(outDir, apiDomain)
            
            // 先下载包装器模块，从中提取实际模块路径
            // 使用索引文件名来存储主模块
            moduleBase := filepath.Dir(modulePath)
            if moduleBase == "." {
                moduleBase = modulePath
            }
            os.MkdirAll(filepath.Join(esmDir, moduleBase), 0755)
            
            // 确定包装器模块的保存路径
            var wrapperPath string
            if strings.HasSuffix(modulePath, "/") || !strings.Contains(modulePath, "/") {
                // 主模块使用 index.js
                wrapperPath = filepath.Join(esmDir, moduleBase, "index.js")
            } else {
                // 子模块使用与模块路径对应的文件名
                filename := filepath.Base(modulePath)
                wrapperPath = filepath.Join(esmDir, moduleBase, filename + ".js")
            }
            
            // 创建模块目录
            if err := os.MkdirAll(filepath.Dir(wrapperPath), 0755); err != nil {
                fmt.Printf("创建子模块目录失败: %v\n", err)
                errChan <- fmt.Errorf("创建子模块目录失败: %v", err)
                return
            }
            
            // 检查是否已下载过此模块，避免重复下载
            downloadedModulesMutex.Lock()
            alreadyDownloaded := downloadedModules[url]
            downloadedModulesMutex.Unlock()
            if alreadyDownloaded {
                fmt.Printf("模块已下载过，跳过: %s\n", url)
                // 标记该URL已经处理过
                downloadedModulesMutex.Lock()
                downloadedModules[url] = true
                downloadedModulesMutex.Unlock()
                return
            }
            
            fmt.Printf("下载包装器模块: %s，保存到: %s\n", url, wrapperPath)
            wrapperContent, err := fetchContent(url)
            if err != nil {
                fmt.Printf("下载包装器模块失败: %v\n", err)
                errChan <- fmt.Errorf("下载包装器模块失败: %v", err)
                return
            }
            
            // 从包装器模块中提取实际模块路径 (在处理内容之前)
            exportRegex := regexp.MustCompile(`(?:import|export\s*\*\s*from|export\s*\{\s*[^}]*\}\s*from)\s*["'](\/[^"']+?)["']`)
            exportMatches := exportRegex.FindAllSubmatch(wrapperContent, -1)
            
            if len(exportMatches) == 0 {
                fmt.Printf("未在包装器模块中找到实际模块路径\n")
                // 先处理包装器模块内容中的路径
                apiDomain := getAPIDomain()
                wrapperContent = processWrapperContent(wrapperContent, apiDomain)
                
                // 保存包装器模块
                if err := os.WriteFile(wrapperPath, wrapperContent, 0644); err != nil {
                    fmt.Printf("保存包装器模块失败: %v\n", err)
                    errChan <- fmt.Errorf("保存包装器模块失败: %v", err)
                    return
                }
                
                // 仍然记为成功，因为我们已经下载了包装器模块
                if strings.Contains(spec, "/") {
                    // 子模块情况，使用子模块的完整路径
                    moduleMap[spec] = "/" + modulePath + ".js"
                    globalModuleMap[spec] = "/" + modulePath + ".js"
                } else {
                    // 主模块情况，使用index.js
                    moduleMap[spec] = "/" + modulePath + "/index.js"
                    globalModuleMap[spec] = "/" + modulePath + "/index.js"
                }
                fmt.Printf("下载成功: %s -> %s\n", spec, wrapperPath)
                return
            }
            
            // 下载所有实际模块
            var actualPaths []string
            for _, match := range exportMatches {
                if len(match) < 2 {
                    continue
                }
                
                actualPath := string(match[1])
                if !strings.HasPrefix(actualPath, "/") {
                    actualPath = "/" + actualPath
                }
                
                // 保存原始路径（带查询参数）用于URL请求
                originalPath := actualPath
                
                // 去除路径中的查询参数，用于文件系统路径
                if strings.Contains(actualPath, "?") {
                    actualPath = strings.Split(actualPath, "?")[0]
                }
                
                // 使用带查询参数的URL进行请求
                actualUrl := apiBaseURL + originalPath
                // 使用不带查询参数的路径作为本地路径
                localPath := filepath.Join(esmDir, actualPath)
                
                // 检查是否已下载过此模块，避免重复下载
                downloadedModulesMutex.Lock()
                alreadyDownloaded := downloadedModules[actualUrl]
                downloadedModulesMutex.Unlock()
                if alreadyDownloaded {
                    fmt.Printf("模块已下载过，跳过: %s\n", actualUrl)
                    actualPaths = append(actualPaths, actualPath)
                    continue
                }
                
                // 标记该URL已经处理过
                downloadedModulesMutex.Lock()
                downloadedModules[actualUrl] = true
                downloadedModulesMutex.Unlock()
                
                fmt.Printf("下载实际模块: %s\n", actualUrl)
                actualContent, err := fetchContent(actualUrl)
                if err != nil {
                    fmt.Printf("下载实际模块失败: %v\n", err)
                    errChan <- fmt.Errorf("下载实际模块失败: %v", err)
                    return
                }
                
                // 创建实际模块目录
                actualDir := filepath.Dir(localPath)
                if err := os.MkdirAll(actualDir, 0755); err != nil {
                    fmt.Printf("创建实际模块目录失败: %v\n", err)
                    errChan <- fmt.Errorf("创建实际模块目录失败: %v", err)
                    return
                }
                
                // 保存实际模块
                if err := os.WriteFile(localPath, actualContent, 0644); err != nil {
                    fmt.Printf("保存实际模块失败: %v\n", err)
                    errChan <- fmt.Errorf("保存实际模块失败: %v", err)
                    return
                }
                
                actualPaths = append(actualPaths, actualPath)
                
                // 在这里立即对下载的实际模块进行递归分析
                // 提取模块中的深层依赖
                depPaths := findDeepDependencies(actualContent)
                // 添加日志：总结发现的依赖数量
                fmt.Printf("分析实际模块中的深层依赖: %s\n", actualPath)
                if len(depPaths) > 0 {
                    fmt.Printf("✅ 共发现 %d 个深层依赖\n", len(depPaths))
                } else {
                    fmt.Printf("⚠️ 未发现任何深层依赖\n")
                    // 取actualContent中头200字节，添加边界检查
                    contentPreview := string(actualContent)
                    if len(contentPreview) > 200 {
                        contentPreview = contentPreview[:200]
                    }
                    fmt.Printf("实际模块内容: %s\n", contentPreview)
                }
                for _, depPath := range depPaths {
                    depUrl := apiBaseURL + depPath
                    downloadedModulesMutex.Lock()
                    alreadyDownloaded := downloadedModules[depUrl]
                    downloadedModulesMutex.Unlock()
                    if !alreadyDownloaded {
                        // 添加日志：开始递归下载
                        fmt.Printf("🚀 开始递归下载依赖: %s\n", depUrl)
                        
                        // 使用新的goroutine递归处理依赖
                        wg.Add(1)
                        go func(depPath, depUrl string) {
                            defer wg.Done()
                            downloadSubModule("", depPath, depUrl, outDir, semaphore, errChan)
                        }(depPath, depUrl)
                    } else {
                        // 添加日志：跳过已下载的依赖
                        fmt.Printf("⏩ 跳过已下载的依赖: %s\n", depUrl)
                    }
                }
                
            }
            
            // 现在处理包装器模块内容中的路径 (在处理所有实际模块后)
            apiDomain := getAPIDomain()
            wrapperContent = processWrapperContent(wrapperContent, apiDomain)
            
            fmt.Printf("包装器模块路径: %s\n", wrapperPath)
            
            // 显示包装器模块内容前200个字符（添加边界检查）
            contentPreview := string(wrapperContent)
            if len(contentPreview) > 200 {
                contentPreview = contentPreview[:200]
            }
            fmt.Printf("包装器模块内容: %s\n", contentPreview)
            
            // 保存包装器模块
            if err := os.WriteFile(wrapperPath, wrapperContent, 0644); err != nil {
                fmt.Printf("保存包装器模块失败: %v\n", err)
                errChan <- fmt.Errorf("保存包装器模块失败: %v", err)
                return
            }
            
            // 子模块不存在实际模块路径的处理
            if len(exportMatches) == 0 {
                fmt.Printf("未在子模块中找到实际模块路径\n")
                globalModuleMap[spec] = "/" + apiBaseURL + "/" + modulePath + ".js"
                return
            }
            
            // 保存映射
            if len(actualPaths) > 0 {
                // 修改映射逻辑
                if strings.Contains(spec, "/") {
                    // 如果是子模块，如react-dom/client，需要保持原有的路径结构
                    moduleMap[spec] = "/" + modulePath + ".js"
                    globalModuleMap[spec] = "/" + modulePath + ".js"
                } else {
                    // 如果是主模块，使用index.js
                    moduleMap[spec] = "/" + modulePath + "/index.js"
                    globalModuleMap[spec] = "/" + modulePath + "/index.js"
                }
                
                // 同时记录实际模块路径用于调试
                moduleMapPath := moduleMap[spec]
                fmt.Printf("已下载实际模块路径: %v，但映射保持使用包装器模块: %s\n", actualPaths, moduleMapPath)
            } else {
                if strings.Contains(spec, "/") {
                    moduleMap[spec] = "/" + modulePath + ".js"
                    globalModuleMap[spec] = "/" + modulePath + ".js"
                } else {
                    moduleMap[spec] = "/" + modulePath + "/index.js"
                    globalModuleMap[spec] = "/" + modulePath + "/index.js"
                }
            }
            
            fmt.Printf("下载成功: %s -> 包装器模块和 %d 个实际模块\n", spec, len(actualPaths))
        }(spec, url)
    }

    // 等待所有下载完成
    fmt.Println("等待所有下载完成...")
    wg.Wait()
    close(errChan)

    // 收集错误
    var errors []string
    for err := range errChan {
        errors = append(errors, err.Error())
    }

    if len(errors) > 0 {
        fmt.Println("下载过程中出现错误:")
        for _, err := range errors {
            fmt.Println(err)
        }
        return fmt.Errorf("下载过程中出现错误:\n%s", strings.Join(errors, "\n"))
    }

    // 5. 复制项目文件到输出目录
    if fileInfo.IsDir() {
        // 如果入口是目录，需要复制整个目录
        fmt.Printf("正在复制项目文件到输出目录...\n")
        err = copyDir(entryPath, outDir)
        if err != nil {
            fmt.Printf("复制项目文件失败: %v\n", err)
            return fmt.Errorf("复制项目文件失败: %v", err)
        }
    } else {
        // 检查是否为前端源文件
        if isFrontendSource {
            // 如果是前端源文件，直接编译该文件
            fmt.Printf("入口文件是前端源文件，直接编译处理: %s\n", actualEntryPath)
            
            // 获取源文件的相对路径
            relPath := filepath.Base(actualEntryPath)
            
            // 编译应用文件
            if err := compileAppFilesWithPath(actualEntryPath, relPath, outDir); err != nil {
                fmt.Printf("编译前端源文件失败: %v\n", err)
                return fmt.Errorf("编译前端源文件失败: %v", err)
            }
            
            fmt.Printf("前端源文件编译完成: %s\n", actualEntryPath)
        } else {
            // 如果是单个HTML文件，复制这个文件
            fmt.Printf("正在复制入口文件到输出目录: %s\n", entryPath)
            targetPath := filepath.Join(outDir, filepath.Base(entryPath))
            if err := os.WriteFile(targetPath, entryContent, 0644); err != nil {
                fmt.Printf("保存入口文件失败: %v\n", err)
                return fmt.Errorf("保存入口文件失败: %v", err)
            }
        }
    }

    // 6. 生成本地 importmap
    fmt.Println("生成本地 importmap...")
    
    // 如果设置了basePath，则修改路径
    localModuleMap := make(map[string]string)
    for spec, path := range moduleMap {
        if basePath != "" && strings.HasPrefix(path, "/") {
            localModuleMap[spec] = basePath + path
        } else {
            localModuleMap[spec] = path
        }
    }
    
    localImportMap := struct {
        Imports map[string]string `json:"imports"`
    }{
        Imports: localModuleMap,
    }
    
    importMapContent, err := json.MarshalIndent(localImportMap, "", "  ")
    if err != nil {
        fmt.Printf("生成本地 importmap 失败: %v\n", err)
        return fmt.Errorf("生成本地 importmap 失败: %v", err)
    }
    
    if err := os.WriteFile(filepath.Join(outDir, "importmap.json"), importMapContent, 0644); err != nil {
        fmt.Printf("保存本地 importmap 失败: %v\n", err)
        return fmt.Errorf("保存本地 importmap 失败: %v", err)
    }
    
    // 7. 修改输出目录中的 index.html (如果存在)
    outputIndexPath := filepath.Join(outDir, "index.html")
    if _, err := os.Stat(outputIndexPath); err == nil && !isFrontendSource {
        fmt.Println("修改输出目录中的 index.html...")
        
        // 读取输出目录中的 index.html
        outputIndexContent, err := os.ReadFile(outputIndexPath)
        if err != nil {
            fmt.Printf("读取输出目录中的 index.html 失败: %v\n", err)
            return fmt.Errorf("读取输出目录中的 index.html 失败: %v", err)
        }
        
        // 替换 importmap
        localHTML := regexp.MustCompile(`<script\s+type="importmap"\s*>[\s\S]*?<\/script>`).
            ReplaceAll(outputIndexContent, []byte(`<script type="importmap" src="./importmap.json"></script>`))
        
        // 如果配置了basePath，需要更新importmap引用
        if basePath != "" {
            // 替换为带basePath的路径
            localHTML = regexp.MustCompile(`<script\s+type="importmap"\s*src="./importmap.json"\s*></script>`).
                ReplaceAll(localHTML, []byte(fmt.Sprintf(`<script type="importmap" src="%s/importmap.json"></script>`, basePath)))
        }
        
        // 8. 处理应用文件 - 查找并处理所有需要编译的本地文件
        fmt.Println("处理应用文件...")
        
        // 找到所有需要编译的文件
        scriptRegex := regexp.MustCompile(`<script\s+[^>]*src="https://esm\.(sh|d8d\.fun)/x"[^>]*href="([^"]+)"[^>]*>(?:</script>)?`)
        scriptMatches := scriptRegex.FindAllSubmatch(localHTML, -1)
        
        fmt.Printf("发现 %d 个应用入口文件\n", len(scriptMatches))
        
        for _, match := range scriptMatches {
            if len(match) < 3 {
                continue
            }
            
            // 获取相对路径
            relPath := string(match[2])
            fmt.Printf("发现入口文件: %s\n", relPath)
            
            // 使用入口的完整路径
            fullPath := filepath.Join(filepath.Dir(indexHtmlPath), relPath)
            fmt.Printf("使用源文件的完整路径: %s\n", fullPath)
            
            // 编译前检查路径
            if _, err := os.Stat(fullPath); os.IsNotExist(err) {
                fmt.Printf("警告: 源文件不存在: %s\n", fullPath)
                return fmt.Errorf("源文件不存在: %s", fullPath)
            }
            
            // 修改compileAppFiles调用，传入入口文件的完整路径和相对路径
            err = compileAppFilesWithPath(fullPath, relPath, outDir)
            if err != nil {
                fmt.Printf("编译应用文件失败: %v\n", err)
                return fmt.Errorf("编译应用文件失败: %v", err)
            }
            
            // 计算编译后文件的路径
            compiledPath := strings.TrimSuffix(relPath, filepath.Ext(relPath)) + ".js"
            // 去掉开头的./，避免./././main.js这样的重复
            compiledPath = strings.TrimPrefix(compiledPath, "./")
            
            // 替换引用，添加basePath支持
            var replacement string
            if basePath != "" {
                replacement = fmt.Sprintf(`<script type="module" src="%s/%s"></script>`, basePath, compiledPath)
            } else {
                replacement = fmt.Sprintf(`<script type="module" src="./%s"></script>`, compiledPath)
            }
            localHTML = scriptRegex.ReplaceAll(localHTML, []byte(replacement))
        }
        
        if err := os.WriteFile(outputIndexPath, localHTML, 0644); err != nil {
            fmt.Printf("保存修改后的 index.html 失败: %v\n", err)
            return fmt.Errorf("保存修改后的 index.html 失败: %v", err)
        }
    }

    fmt.Printf("下载完成！所有文件已保存到 %s 目录\n", outDir)
    return nil
}

func fetchContent(url string) ([]byte, error) {
    // 创建一个自定义的 HTTP 客户端，设置不自动重定向
    client := &http.Client{
        CheckRedirect: func(req *http.Request, via []*http.Request) error {
            // 不自动重定向，而是返回重定向响应
            return http.ErrUseLastResponse
        },
    }
    
    // 1. 获取文件内容
    resp, err := client.Get(url)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    // 处理重定向
    if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusMovedPermanently || 
       resp.StatusCode == http.StatusTemporaryRedirect || resp.StatusCode == http.StatusPermanentRedirect {
        redirectURL, err := resp.Location()
        if err != nil {
            return nil, fmt.Errorf("获取重定向URL失败: %v", err)
        }
        fmt.Printf("发现重定向: %s -> %s\n", url, redirectURL.String())
        return fetchContent(redirectURL.String())
    }
    
    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("HTTP 错误: %d %s - %s", resp.StatusCode, resp.Status, string(body))
    }
    
    return io.ReadAll(resp.Body)
}

// 复制目录
func copyDir(src, dst string) error {
    // 获取源目录信息
    srcInfo, err := os.Stat(src)
    if err != nil {
        return err
    }
    
    // 创建目标目录
    if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
        return err
    }
    
    // 读取源目录内容
    entries, err := os.ReadDir(src)
    if err != nil {
        return err
    }
    
    // 遍历源目录内容
    for _, entry := range entries {
        srcPath := filepath.Join(src, entry.Name())
        dstPath := filepath.Join(dst, entry.Name())
        
        // 获取API域名作为目录名
        apiDomain := getAPIDomain()
        
        // 如果与API域名匹配，跳过（该目录将由下载过程创建）
        if entry.Name() == apiDomain || entry.Name() == "esm.sh" {
            fmt.Printf("跳过API目录: %s\n", entry.Name())
            continue
        }
        
        // 跳过 TypeScript 和 JSX 源文件，这些文件会被编译
        if !entry.IsDir() {
            ext := filepath.Ext(entry.Name())
            if ext == ".tsx" || ext == ".ts" || ext == ".jsx" {
                fmt.Printf("跳过源文件: %s\n", srcPath)
                continue
            }
        }
        
        // 如果是目录，递归复制
        if entry.IsDir() {
            if err := copyDir(srcPath, dstPath); err != nil {
                return err
            }
        } else {
            // 复制文件
            if err := copyFile(srcPath, dstPath); err != nil {
                return err
            }
        }
    }
    
    return nil
}

// 复制文件
func copyFile(src, dst string) error {
    // 打开源文件
    srcFile, err := os.Open(src)
    if err != nil {
        return err
    }
    defer srcFile.Close()
    
    // 创建目标文件
    dstFile, err := os.Create(dst)
    if err != nil {
        return err
    }
    defer dstFile.Close()
    
    // 复制内容
    _, err = io.Copy(dstFile, srcFile)
    if err != nil {
        return err
    }
    
    // 获取源文件权限
    srcInfo, err := os.Stat(src)
    if err != nil {
        return err
    }
    
    // 设置目标文件权限
    return os.Chmod(dst, srcInfo.Mode())
}

// 使用 esm.sh 转译 API 编译文件
func compileFile(content string, filename string) (string, error) {
    // 检查文件类型
    fileExt := filepath.Ext(filename)
    
    // 对于CSS文件，直接返回原内容，不进行编译
    if fileExt == ".css" {
        return content, nil
    }
    
    // 确定文件类型
    var lang string
    switch fileExt {
    case ".ts":
        lang = "ts"
    case ".tsx":
        lang = "tsx"
    case ".jsx":
        lang = "jsx"
    case ".js":
        lang = "js"
    default:
        return "", fmt.Errorf("不支持的文件类型: %s", fileExt)
    }
    
    // 提取域名部分，用于后续处理
    apiDomain := strings.TrimPrefix(strings.TrimPrefix(apiBaseURL, "https://"), "http://")
    
    // 构建自定义 importmap，基于已下载的模块
    customImportMap := make(map[string]string)
    for moduleName, localPath := range globalModuleMap {
        customImportMap[moduleName] = localPath
        
        // 添加常见的子模块映射
        // if moduleName == "react" {
        //     customImportMap["react/jsx-runtime"] = "/" + apiDomain + "/react/jsx-runtime"
        // } else if moduleName == "react-dom" {
        //     customImportMap["react-dom/client"] = "/" + apiDomain + "/react-dom/client"
        // }
    }
    
    importMapBytes, err := json.Marshal(map[string]map[string]string{
        "imports": customImportMap,
    })
    if err != nil {
        return "", fmt.Errorf("创建 importmap 失败: %v", err)
    }
    
    // 构建请求
    transformRequest := struct {
        Code          string          `json:"code"`
        Filename      string          `json:"filename"`
        Lang          string          `json:"lang"`
        Target        string          `json:"target"`
        ImportMap     json.RawMessage `json:"importMap"`
        JsxImportSource string        `json:"jsxImportSource,omitempty"`
        Minify        bool            `json:"minify"`
    }{
        Code:          content,
        Filename:      filename,
        Lang:          lang,
        Target:        "es2022",
        ImportMap:     importMapBytes,
        Minify:        minify,
    }
    
    // 如果是 JSX/TSX，添加 JSX 导入源
    if lang == "tsx" || lang == "jsx" {
        transformRequest.JsxImportSource = "react"
    }
    
    // 序列化请求
    reqBody, err := json.Marshal(transformRequest)
    if err != nil {
        return "", fmt.Errorf("序列化请求失败: %v", err)
    }
    
    // 发送请求
    resp, err := http.Post(apiBaseURL + "/transform", "application/json", strings.NewReader(string(reqBody)))
    if err != nil {
        return "", fmt.Errorf("发送请求失败: %v", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return "", fmt.Errorf("请求失败: %d %s - %s", resp.StatusCode, resp.Status, string(body))
    }
    
    // 解析响应
    var result struct {
        Code string `json:"code"`
        Map  string `json:"map"`
    }
    
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return "", fmt.Errorf("解析响应失败: %v", err)
    }
    
    // 进一步处理编译后的代码，将引用替换为本地路径
    compiledCode := result.Code
    
    // 修复重复的 /esm.sh 路径问题
    // 例如将 "/esm.sh/esm.sh/react/jsx-runtime" 替换为 "/esm.sh/react/jsx-runtime"
    duplicatePathRegex := regexp.MustCompile(`from\s+["'](/` + apiDomain + `/` + apiDomain + `/([^"']+))["']`)
    compiledCode = duplicatePathRegex.ReplaceAllString(compiledCode, `from "/` + apiDomain + `/$2"`)
    
    // 添加路径替换，处理相对路径引用
    // 例如将 "/react-dom@19.1.0/es2022/react-dom.mjs" 替换为 "/esm.sh/react-dom@19.1.0/es2022/react-dom.mjs"
    pathRegex := regexp.MustCompile(`from\s+["'](\/([@\w\d\.-]+)\/[^"']+)["']`)
    if basePath != "" {
        // 如果设置了basePath，添加前缀
        compiledCode = pathRegex.ReplaceAllString(compiledCode, `from "` + basePath + `/` + apiDomain + `$1"`)
    } else {
        compiledCode = pathRegex.ReplaceAllString(compiledCode, `from "/` + apiDomain + `$1"`)
    }
    
    // 处理没有from的裸导入语句 (例如 import "/dayjs@1.11.13/locale/zh-cn.js")
    bareImportRegex := regexp.MustCompile(`import\s+["'](\/([@\w\d\.-]+)\/[^"']+)["'];`)
    if basePath != "" {
        // 如果设置了basePath，添加前缀
        compiledCode = bareImportRegex.ReplaceAllString(compiledCode, `import "` + basePath + `/` + apiDomain + `$1";`)
    } else {
        compiledCode = bareImportRegex.ReplaceAllString(compiledCode, `import "/` + apiDomain + `$1";`)
    }
    
    // 替换本地相对路径引用的扩展名（.tsx/.ts/.jsx -> .js）
    localImportRegex := regexp.MustCompile(`from\s+["'](\.[^"']+)(\.tsx|\.ts|\.jsx)["']`)
    compiledCode = localImportRegex.ReplaceAllString(compiledCode, `from "$1.js"`)
    
    return compiledCode, nil
}

// 下载子模块
func downloadSubModule(parentModule, subModule, url, outDir string, semaphore chan struct{}, errChan chan error) {
    // 检查是否已下载过此模块
    moduleKey := url
    downloadedModulesMutex.Lock()
    alreadyDownloaded := downloadedModules[moduleKey]
    downloadedModulesMutex.Unlock()
    if alreadyDownloaded {
        fmt.Printf("模块已下载过，跳过: %s\n", url)
        return
    }
    downloadedModulesMutex.Lock()
    downloadedModules[moduleKey] = true
    downloadedModulesMutex.Unlock()
    
    fmt.Printf("准备下载子模块: %s\n", subModule)
    
    semaphore <- struct{}{}
    defer func() { <-semaphore }()
    
    // 解析URL中的模块路径
    moduleRegex := regexp.MustCompile(`https://esm\.(sh|d8d\.fun)/(.+)`)
    matches := moduleRegex.FindStringSubmatch(url)
    
    var modulePath string
    if len(matches) > 2 {
        modulePath = matches[2]
        // 处理URL中的查询参数
        if strings.Contains(modulePath, "?") {
            modulePath = strings.Split(modulePath, "?")[0]
        }
    } else {
        modulePath = subModule
        // 处理subModule中可能的查询参数
        if strings.Contains(modulePath, "?") {
            modulePath = strings.Split(modulePath, "?")[0]
        }
    }
    
    // 提取域名部分，用于后续处理
    apiDomain := strings.TrimPrefix(strings.TrimPrefix(apiBaseURL, "https://"), "http://")
    
    // 使用传入的输出目录和API域名
    esmDir := filepath.Join(outDir, apiDomain)
    
    // 确保子模块文件命名正确
    moduleDir := filepath.Dir(modulePath)
    
    // 确定包装器模块的保存路径
    var wrapperPath string
    if strings.HasSuffix(modulePath, "/") || !strings.Contains(modulePath, "/") {
        // 主模块使用index.js
        wrapperPath = filepath.Join(esmDir, moduleDir, "index.js")
    } else {
        // 子模块使用对应的文件名
        fileName := filepath.Base(modulePath)
        wrapperPath = filepath.Join(esmDir, moduleDir, fileName+".js")
    }
    
    // 创建模块目录
    if err := os.MkdirAll(filepath.Dir(wrapperPath), 0755); err != nil {
        fmt.Printf("创建子模块目录失败: %v\n", err)
        errChan <- fmt.Errorf("创建子模块目录失败: %v", err)
        return
    }
    
    // 下载包装器模块
    fmt.Printf("下载子模块: %s，保存到: %s\n", url, wrapperPath)
    wrapperContent, err := fetchContent(url)
    if err != nil {
        fmt.Printf("下载子模块失败: %v\n", err)
        errChan <- fmt.Errorf("下载子模块失败: %v", err)
        return
    }
    
    // 从包装器模块中提取实际模块路径（先提取再处理内容）
    exportRegex := regexp.MustCompile(`(?:import|export\s*\*\s*from|export\s*\{\s*[^}]*\}\s*from)\s*["'](\/[^"']+?)["']`)
    exportMatches := exportRegex.FindAllSubmatch(wrapperContent, -1)
    
    if len(exportMatches) == 0 {
        fmt.Printf("未在子模块中找到实际模块路径\n")
        
        // 处理包装器模块内容中的路径
        wrapperContent = processWrapperContent(wrapperContent, apiDomain)
        
        // 保存包装器模块
        if err := os.WriteFile(wrapperPath, wrapperContent, 0644); err != nil {
            fmt.Printf("保存子模块失败: %v\n", err)
            errChan <- fmt.Errorf("保存子模块失败: %v", err)
            return
        }
        
        globalModuleMap[subModule] = "/" + modulePath + ".js"
        
        // 分析包装器模块中的依赖
        bareImports := findBareImports(wrapperContent)
        for _, dep := range bareImports {
            if !isLocalPath(dep) && !strings.HasPrefix(dep, "/") {
                // 构建子依赖的URL
                depURL := constructDependencyURL(dep, apiBaseURL)
                downloadedModulesMutex.Lock()
                alreadyDownloaded := downloadedModules[depURL]
                downloadedModulesMutex.Unlock()
                if depURL != "" && !alreadyDownloaded {
                    // 递归下载子依赖
                    fmt.Printf("📦 递归下载裸依赖: %s -> %s\n", dep, depURL)
                    go downloadSubModule("", dep, depURL, outDir, semaphore, errChan)
                } else if depURL != "" {
                    fmt.Printf("⏩ 跳过已下载的裸依赖: %s\n", depURL)
                }
            }
        }
        
        return
    }
    
    // 下载所有实际模块
    var actualPaths []string
    for _, match := range exportMatches {
        if len(match) < 2 {
            continue
        }
        
        actualPath := string(match[1])
        if !strings.HasPrefix(actualPath, "/") {
            actualPath = "/" + actualPath
        }
        
        // 保存原始路径（带查询参数）用于URL请求
        originalPath := actualPath
        
        // 去除路径中的查询参数，用于文件系统路径
        if strings.Contains(actualPath, "?") {
            actualPath = strings.Split(actualPath, "?")[0]
        }
        
        // 使用带查询参数的URL进行请求
        actualUrl := apiBaseURL + originalPath
        // 使用不带查询参数的路径作为本地路径
        localPath := filepath.Join(esmDir, actualPath)
        
        fmt.Printf("下载子模块实际文件: %s\n", actualUrl)
        actualContent, err := fetchContent(actualUrl)
        if err != nil {
            fmt.Printf("下载子模块实际文件失败: %v\n", err)
            errChan <- fmt.Errorf("下载子模块实际文件失败: %v", err)
            return
        }
        
        // 创建实际模块目录
        actualDir := filepath.Dir(localPath)
        if err := os.MkdirAll(actualDir, 0755); err != nil {
            fmt.Printf("创建子模块实际文件目录失败: %v\n", err)
            errChan <- fmt.Errorf("创建子模块实际文件目录失败: %v", err)
            return
        }
        
        // 保存实际模块
        if err := os.WriteFile(localPath, actualContent, 0644); err != nil {
            fmt.Printf("保存子模块实际文件失败: %v\n", err)
            errChan <- fmt.Errorf("保存子模块实际文件失败: %v", err)
            return
        }
        
        actualPaths = append(actualPaths, actualPath)
        
        // 分析实际模块中的裸依赖
        bareImports := findBareImports(actualContent)
        for _, dep := range bareImports {
            if !isLocalPath(dep) && !strings.HasPrefix(dep, "/") {
                // 构建子依赖的URL
                depURL := constructDependencyURL(dep, apiBaseURL)
                downloadedModulesMutex.Lock()
                alreadyDownloaded := downloadedModules[depURL]
                downloadedModulesMutex.Unlock()
                if depURL != "" && !alreadyDownloaded {
                    // 递归下载子依赖
                    fmt.Printf("📦 递归下载裸依赖: %s -> %s\n", dep, depURL)
                    go downloadSubModule("", dep, depURL, outDir, semaphore, errChan)
                } else if depURL != "" {
                    fmt.Printf("⏩ 跳过已下载的裸依赖: %s\n", depURL)
                }
            }
        }
        
        // 分析实际模块中的深层依赖（直接路径引用）
        fmt.Printf("分析实际模块中的深层依赖: %s\n", actualPath)
        depPaths := findDeepDependencies(actualContent)
        for _, depPath := range depPaths {
            depUrl := apiBaseURL + depPath
            downloadedModulesMutex.Lock()
            alreadyDownloaded := downloadedModules[depUrl]
            downloadedModulesMutex.Unlock()
            if !alreadyDownloaded {
                fmt.Printf("🔍 递归下载深层依赖: %s\n", depUrl)
                go downloadSubModule("", depPath, depUrl, outDir, semaphore, errChan)
            } else {
                fmt.Printf("⏩ 跳过已下载的深层依赖: %s\n", depUrl)
            }
        }
    }
    
    // 处理包装器模块内容中的路径（在提取和下载实际模块后）
    wrapperContent = processWrapperContent(wrapperContent, apiDomain)
    
    // 保存包装器模块
    if err := os.WriteFile(wrapperPath, wrapperContent, 0644); err != nil {
        fmt.Printf("保存子模块失败: %v\n", err)
        errChan <- fmt.Errorf("保存子模块失败: %v", err)
        return
    }
    
    // 保存映射到包装器模块而非实际模块
    globalModuleMap[subModule] = "/" + modulePath + ".js"
    fmt.Printf("已下载子模块实际路径: %v，但映射保持使用包装器模块: /%s\n", 
               actualPaths, modulePath + ".js")
    
    fmt.Printf("子模块下载成功: %s\n", subModule)
}

// 判断是否为本地路径
func isLocalPath(path string) bool {
    return strings.HasPrefix(path, ".") || strings.HasPrefix(path, "/")
}

// 查找模块中的裸导入（不带路径前缀的导入）
func findBareImports(content []byte) []string {
    // 使用正则表达式找出所有import语句中的裸导入
    importRegex := regexp.MustCompile(`(?:import|export\s*\*\s*from|export\s*\{\s*[^}]*\}\s*from)\s*["']([^"'./][^"']+)["']`)
    matches := importRegex.FindAllSubmatch(content, -1)
    
    var bareImports []string
    for _, match := range matches {
        if len(match) >= 2 {
            bareImport := string(match[1])
            // 排除已有的URL格式导入
            if !strings.HasPrefix(bareImport, "http") {
                bareImports = append(bareImports, bareImport)
            }
        }
    }
    
    return bareImports
}

// 构建依赖的URL
func constructDependencyURL(dep, apiBaseURL string) string {
    // 处理可能的子模块
    var baseModule, subModule string
    if idx := strings.Index(dep, "/"); idx != -1 {
        baseModule = dep[:idx]
        subModule = dep[idx+1:]
    } else {
        baseModule = dep
        subModule = ""
    }
    
    // 查找依赖是否已在importmap中
    for spec, url := range globalModuleMap {
        if spec == dep {
            return url
        }
    }
    
    // 从已下载模块中查找版本信息
    var version string
    for url := range downloadedModules {
        versionRegex := regexp.MustCompile(`/` + regexp.QuoteMeta(baseModule) + `@([\d\.]+)`)
        matches := versionRegex.FindStringSubmatch(url)
        if len(matches) > 1 {
            version = matches[1]
            break
        }
    }
    
    if version == "" {
        // 无法确定版本，使用最新版本
        version = "latest"
    }
    
    if subModule == "" {
        return fmt.Sprintf("%s/%s@%s", apiBaseURL, baseModule, version)
    } else {
        return fmt.Sprintf("%s/%s@%s/%s", apiBaseURL, baseModule, version, subModule)
    }
}

// 编译应用文件并处理其所有本地依赖
func compileAppFilesWithPath(fullPath, relPath, outDir string) error {
    // 获取源文件的base目录，用于查找相对导入
    baseDir := filepath.Dir(fullPath)
    
    // 维护已编译文件集合，避免重复编译
    compiledFiles := make(map[string]bool)
    
    // 使用队列处理所有需要编译的文件
    queue := []string{relPath}
    
    fmt.Printf("源文件根目录: %s\n", baseDir)
    
    for len(queue) > 0 {
        // 取出队列中的第一个文件
        currentFile := queue[0]
        queue = queue[1:]
        
        // 如果文件已经被编译过，则跳过
        if compiledFiles[currentFile] {
            continue
        }
        
        var srcPath string
        
        // 如果当前处理的是入口文件，直接使用提供的完整路径
        if currentFile == relPath {
            srcPath = fullPath
            fmt.Printf("使用入口文件的完整路径: %s\n", srcPath)
        } else {
            // 对于其他文件，计算相对于baseDir的路径
            // 去掉前缀的./以避免路径计算错误
            cleanCurrentFile := strings.TrimPrefix(currentFile, "./")
            
            // 确保不重复添加目录部分
            if filepath.IsAbs(cleanCurrentFile) || strings.HasPrefix(cleanCurrentFile, baseDir) {
                srcPath = cleanCurrentFile
            } else {
                // 否则才拼接路径
                srcPath = filepath.Join(baseDir, cleanCurrentFile)
            }
            
            fmt.Printf("计算依赖文件路径: %s\n", srcPath)
        }
        
        // 检查文件是否存在
        if _, err := os.Stat(srcPath); os.IsNotExist(err) {
            // 尝试其他可能的路径
            cleanCurrentFile := strings.TrimPrefix(currentFile, "./")
            altPath := filepath.Join(filepath.Dir(baseDir), cleanCurrentFile)
            if _, err := os.Stat(altPath); err == nil {
                srcPath = altPath
                fmt.Printf("使用替代路径: %s\n", srcPath)
            } else {
                return fmt.Errorf("找不到源文件: %s", srcPath)
            }
        }
        
        // 编译后的文件保存在输出目录
        outputPath := filepath.Join(outDir, strings.TrimSuffix(currentFile, filepath.Ext(currentFile)) + ".js")
        fmt.Printf("编译文件: %s -> %s\n", srcPath, outputPath)
        
        // 确保输出目录存在
        if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
            return fmt.Errorf("创建输出目录失败 %s: %v", outputPath, err)
        }
        
        // 检查文件类型
        fileExt := filepath.Ext(currentFile)
        
        // 对于不需要编译的文件类型，直接复制
        if fileExt == ".css" || fileExt == ".svg" || fileExt == ".json" {
            // 复制文件
            if err := copyFile(srcPath, filepath.Join(outDir, currentFile)); err != nil {
                return fmt.Errorf("复制资源文件失败 %s: %v", srcPath, err)
            }
            
            // 标记为已处理
            compiledFiles[currentFile] = true
            fmt.Printf("复制非模块文件: %s -> %s\n", srcPath, filepath.Join(outDir, currentFile))
            continue
        }
        
        // 读取源文件内容
        srcContent, err := os.ReadFile(srcPath)
        if err != nil {
            return fmt.Errorf("读取源文件失败 %s: %v", srcPath, err)
        }
        
        // 编译文件
        compiledContent, err := compileFile(string(srcContent), currentFile)
        if err != nil {
            return fmt.Errorf("编译文件失败 %s: %v", currentFile, err)
        }
        
        // 写入编译后的文件
        if err := os.WriteFile(outputPath, []byte(compiledContent), 0644); err != nil {
            return fmt.Errorf("保存编译后的文件失败 %s: %v", outputPath, err)
        }
        
        // 标记该文件已编译
        compiledFiles[currentFile] = true
        
        // 查找文件中的本地导入
        imports := findLocalImports(string(srcContent))
        for _, imp := range imports {
            // 解析导入路径
            importDir := filepath.Dir(currentFile)
            resolvedPath := resolveImportPath(baseDir, imp)
            fmt.Printf("发现本地依赖: 从 %s 导入 %s -> 解析为 %s\n", importDir, imp, resolvedPath)
            
            // 优先检查当前目录的相对路径
            relativeToCurrentFile := filepath.Join(filepath.Dir(srcPath), strings.TrimPrefix(imp, "./"))
            if _, err := os.Stat(relativeToCurrentFile); err == nil {
                resolvedPath = filepath.Clean(filepath.Join(filepath.Dir(currentFile), strings.TrimPrefix(imp, "./")))
                fmt.Printf("使用相对当前文件的路径: %s\n", resolvedPath)
            }
            
            // 添加到队列
            if !compiledFiles[resolvedPath] {
                queue = append(queue, resolvedPath)
            }
        }
    }
    
    return nil
}

// 查找文件中的本地导入
func findLocalImports(content string) []string {
    // 匹配所有相对导入，如 './Component.tsx', '../utils/helper.ts'
    importRegex := regexp.MustCompile(`(?:import|from)\s+['"](\.[^'"]+)['"]`)
    matches := importRegex.FindAllStringSubmatch(content, -1)
    
    var imports []string
    for _, match := range matches {
        if len(match) > 1 {
            importPath := match[1]
            fmt.Printf("原始导入路径: %s\n", importPath)
            
            // 处理可能的路径分隔符不一致问题
            importPath = filepath.FromSlash(importPath)
            
            imports = append(imports, importPath)
        }
    }
    
    return imports
}

// 解析导入路径
func resolveImportPath(baseDir, importPath string) string {
    // 如果importPath包含baseDir，则直接使用importPath
    if strings.HasPrefix(importPath, baseDir) {
        importPath = strings.TrimPrefix(importPath, baseDir)
        importPath = strings.TrimPrefix(importPath, "/")
    }
    
    // 处理扩展名
    ext := filepath.Ext(importPath)
    if ext == "" {
        // 无扩展名的情况，尝试常见的扩展名
        for _, possibleExt := range []string{".tsx", ".ts", ".jsx", ".js"} {
            possiblePath := importPath + possibleExt
            fullPath := filepath.Join(baseDir, possiblePath)
            if _, err := os.Stat(fullPath); err == nil {
                importPath = possiblePath
                break
            }
        }
    }
    
    // 返回相对于项目根目录的路径
    return filepath.Clean(filepath.Join(baseDir, importPath))
}

// 自动添加常用的React相关子模块
func addReactJsxRuntime(data *struct{ Imports map[string]string `json:"imports"` }) {
    // 检查并添加 react/jsx-runtime
    addReactSubmodule(data, "react", "jsx-runtime")
    
    // 检查并添加 react-dom/client
    addReactSubmodule(data, "react-dom", "client")
}

// 添加React相关子模块的通用函数
func addReactSubmodule(data *struct{ Imports map[string]string `json:"imports"` }, baseModule, subModule string) {
    // 检查是否存在基础模块
    baseUrl, baseExists := data.Imports[baseModule]
    if !baseExists {
        fmt.Printf("未找到%s模块，不添加%s/%s子模块\n", baseModule, baseModule, subModule)
        return
    }
    
    // 子模块完整名称
    fullSubModuleName := baseModule + "/" + subModule
    
    // 检查是否已经包含子模块
    if _, exists := data.Imports[fullSubModuleName]; !exists {
        fmt.Printf("自动添加%s子模块\n", fullSubModuleName)
        
        // 从基础URL中提取版本信息
        versionRegex := regexp.MustCompile(baseModule + `@([\d\.]+)`)
        matches := versionRegex.FindStringSubmatch(baseUrl)
        
        var version string
        if len(matches) > 1 {
            version = matches[1]
            fmt.Printf("检测到%s版本: %s\n", baseModule, version)
            
            // 根据版本构造子模块URL
            subModuleUrl := strings.Replace(baseUrl, baseModule+"@"+version, baseModule+"@"+version+"/"+subModule, 1)
            data.Imports[fullSubModuleName] = subModuleUrl
            fmt.Printf("添加%s模块: %s\n", fullSubModuleName, subModuleUrl)
        } else {
            // 如果无法确定版本，使用与基础模块相同的URL结构
            fmt.Printf("无法从URL确定%s版本，使用与%s相同的URL结构\n", baseModule, baseModule)
            
            // 构造子模块URL，替换路径部分
            subModuleUrl := strings.Replace(baseUrl, baseModule, baseModule+"/"+subModule, 1)
            data.Imports[fullSubModuleName] = subModuleUrl
            fmt.Printf("添加%s模块: %s\n", fullSubModuleName, subModuleUrl)
        }
    }
}

// 处理包装器模块的内容，修正其中的导入路径
func processWrapperContent(content []byte, apiDomain string) []byte {
    contentStr := string(content)

    // 处理裸导入路径，添加API域名前缀
    // 如 import "/react-dom@19.0.0/es2022/react-dom.mjs" 
    // 变为 import "/esm.d8d.fun/react-dom@19.0.0/es2022/react-dom.mjs"
    importRegex := regexp.MustCompile(`(import|export\s*\*\s*from|export\s*\{\s*[^}]*\}\s*from)\s*["'](\/.+?)["']`)
    contentStr = importRegex.ReplaceAllStringFunc(contentStr, func(match string) string {
        parts := importRegex.FindStringSubmatch(match)
        if len(parts) >= 3 {
            originalPath := parts[2]
            
            // 检查路径是否已经包含API域名
            if !strings.Contains(originalPath, "/"+apiDomain+"/") {
                // 分离路径和查询参数
                path := originalPath
                var query string
                if strings.Contains(path, "?") {
                    pathParts := strings.SplitN(path, "?", 2)
                    path = pathParts[0]
                    query = "?" + pathParts[1]
                } else {
                    query = ""
                }
                
                // 替换为带API域名的路径
                var newPath string
                if basePath != "" && !strings.HasPrefix(path, basePath) {
                    // 如果设置了basePath，添加前缀
                    newPath = basePath + "/" + apiDomain + path + query
                } else {
                    newPath = "/" + apiDomain + path + query
                }
                return strings.Replace(match, originalPath, newPath, 1)
            }
        }
        return match
    })

    return []byte(contentStr)
}

// 从模块内容中找出深层依赖
func findDeepDependencies(content []byte) []string {
    // 提取形如 "/react-dom@19.0.0/es2022/react-dom.mjs" 的依赖路径
    // import*as __0$ from"/react@19.0.0/es2022/react.mjs";
    // dependencyRegex := regexp.MustCompile(`(?:import|export\s*\*\s*from|export\s*\{\s*[^}]*\}\s*from)\s*["'](\/[@\w\d\.\-]+\/[^"']+)["']`)
    dependencyRegex := regexp.MustCompile(`(?:import\s*\*?\s*as\s*[^"']*\s*from|import\s*\{[^}]*\}\s*from|import|export\s*\*\s*from|export\s*\{\s*[^}]*\}\s*from)\s*["'](\/[@\w\d\.\-]+\/[^"']+)["']`)
    matches := dependencyRegex.FindAllSubmatch(content, -1)
    
    var deps []string
    seen := make(map[string]bool)
    
    // 添加日志：显示正在分析的内容长度
    fmt.Printf("正在分析模块内容，长度: %d 字节\n", len(content))
    
    for _, match := range matches {
        if len(match) >= 2 {
            dep := string(match[1])
            if !seen[dep] {
                seen[dep] = true
                deps = append(deps, dep)
                // 添加日志：每发现一个依赖就记录
                fmt.Printf("🔍 发现依赖: %s\n", dep)
            }
        }
    }
    
    
    return deps
} 