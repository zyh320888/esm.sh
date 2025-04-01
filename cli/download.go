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

func DownloadDependencies(args []string) error {
    fmt.Println("开始执行下载命令...")
    
    // 初始化全局模块映射
    globalModuleMap = make(map[string]string)
    
    if len(args) < 1 {
        return fmt.Errorf("请指定入口文件或目录")
    }

    entryPath := args[0]
    outDir := "dist"
    
    fmt.Printf("入口路径: %s\n", entryPath)
    
    // 从参数中获取输出目录
    for i := 1; i < len(args); i++ {
        if args[i] == "--out-dir" && i+1 < len(args) {
            outDir = args[i+1]
            fmt.Printf("输出目录: %s\n", outDir)
            break
        }
    }

    // 检查入口是文件还是目录
    fileInfo, err := os.Stat(entryPath)
    if err != nil {
        fmt.Printf("获取入口信息失败: %v\n", err)
        return fmt.Errorf("获取入口信息失败: %v", err)
    }

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
    } else {
        // 如果是文件，直接使用
        indexHtmlPath = entryPath
    }

    // 1. 读取入口文件
    fmt.Printf("正在读取入口文件: %s\n", indexHtmlPath)
    entryContent, err := os.ReadFile(indexHtmlPath)
    if err != nil {
        fmt.Printf("读取入口文件失败: %v\n", err)
        return fmt.Errorf("读取入口文件失败: %v", err)
    }
    fmt.Println("入口文件读取成功")

    // 2. 解析 importmap
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
    
    var importMapData struct {
        Imports map[string]string `json:"imports"`
    }
    
    if err := json.Unmarshal(importMapJson, &importMapData); err != nil {
        fmt.Printf("解析 importmap 失败: %v\n", err)
        return fmt.Errorf("解析 importmap 失败: %v", err)
    }
    
    if importMapData.Imports == nil {
        fmt.Println("importmap 不包含有效的 imports 字段")
        return fmt.Errorf("importmap 不包含有效的 imports 字段")
    }
    
    fmt.Printf("解析到的 importmap: %v\n", importMapData.Imports)

    // 3. 创建输出目录
    fmt.Printf("正在创建输出目录: %s\n", outDir)
    if err := os.MkdirAll(outDir, 0755); err != nil {
        fmt.Printf("创建输出目录失败: %v\n", err)
        return fmt.Errorf("创建输出目录失败: %v", err)
    }
    
    // 创建 esm.sh 目录
    esmDir := filepath.Join(outDir, "esm.sh")
    if err := os.MkdirAll(esmDir, 0755); err != nil {
        fmt.Printf("创建 esm.sh 目录失败: %v\n", err)
        return fmt.Errorf("创建 esm.sh 目录失败: %v", err)
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
            moduleRegex := regexp.MustCompile(`https://esm\.sh/(.+)`)
            matches := moduleRegex.FindStringSubmatch(url)
            
            var modulePath string
            if len(matches) > 1 {
                modulePath = matches[1]
            } else {
                // 如果URL不是esm.sh格式的，使用spec作为模块路径
                modulePath = spec
            }
            
            // 创建模块目录
            moduleDir := filepath.Join(esmDir, filepath.Dir(modulePath))
            if err := os.MkdirAll(moduleDir, 0755); err != nil {
                fmt.Printf("创建模块目录失败: %v\n", err)
                errChan <- fmt.Errorf("创建模块目录失败: %v", err)
                return
            }
            
            // 先下载包装器模块，从中提取实际模块路径
            wrapperPath := filepath.Join(esmDir, modulePath+".js")
            
            fmt.Printf("下载包装器模块: %s\n", url)
            wrapperContent, err := fetchContent(url)
            if err != nil {
                fmt.Printf("下载包装器模块失败: %v\n", err)
                errChan <- fmt.Errorf("下载包装器模块失败: %v", err)
                return
            }
            
            // 保存包装器模块
            if err := os.WriteFile(wrapperPath, wrapperContent, 0644); err != nil {
                fmt.Printf("保存包装器模块失败: %v\n", err)
                errChan <- fmt.Errorf("保存包装器模块失败: %v", err)
                return
            }
            
            // 从包装器模块中提取实际模块路径
            exportRegex := regexp.MustCompile(`["']([^"']+\.mjs)["']`)
            exportMatches := exportRegex.FindAllSubmatch(wrapperContent, -1)
            
            if len(exportMatches) == 0 {
                fmt.Printf("未在包装器模块中找到实际模块路径\n")
                // 仍然记为成功，因为我们已经下载了包装器模块
                moduleMap[spec] = "/" + modulePath + ".js"
                globalModuleMap[spec] = "/" + modulePath + ".js"
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
                
                actualUrl := "https://esm.sh" + actualPath
                localPath := filepath.Join(esmDir, actualPath)
                
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
            }
            
            // 保存映射
            if len(actualPaths) > 0 {
                moduleMap[spec] = actualPaths[0]
                globalModuleMap[spec] = actualPaths[0]
                
                // 下载常见的子模块
                if spec == "react" {
                    // 下载 react/jsx-runtime
                    downloadSubModule(spec, "react/jsx-runtime", "https://esm.sh/react/jsx-runtime", outDir, semaphore, errChan)
                } else if spec == "react-dom" {
                    // 下载 react-dom/client
                    downloadSubModule(spec, "react-dom/client", "https://esm.sh/react-dom/client", outDir, semaphore, errChan)
                }
            } else {
                moduleMap[spec] = "/" + modulePath + ".js"
                globalModuleMap[spec] = "/" + modulePath + ".js"
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
        // 如果是单个文件，只复制这个文件
        fmt.Printf("正在复制入口文件到输出目录: %s\n", entryPath)
        targetPath := filepath.Join(outDir, filepath.Base(entryPath))
        if err := os.WriteFile(targetPath, entryContent, 0644); err != nil {
            fmt.Printf("保存入口文件失败: %v\n", err)
            return fmt.Errorf("保存入口文件失败: %v", err)
        }
    }

    // 6. 生成本地 importmap
    fmt.Println("生成本地 importmap...")
    
    localImportMap := struct {
        Imports map[string]string `json:"imports"`
    }{
        Imports: moduleMap,
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
    
    // 7. 修改输出目录中的 index.html
    fmt.Println("修改输出目录中的 index.html...")
    
    // 读取输出目录中的 index.html
    outputIndexPath := filepath.Join(outDir, "index.html")
    outputIndexContent, err := os.ReadFile(outputIndexPath)
    if err != nil {
        fmt.Printf("读取输出目录中的 index.html 失败: %v\n", err)
        return fmt.Errorf("读取输出目录中的 index.html 失败: %v", err)
    }
    
    // 替换 importmap
    localHTML := regexp.MustCompile(`<script\s+type="importmap"\s*>[\s\S]*?<\/script>`).
        ReplaceAll(outputIndexContent, []byte(`<script type="importmap" src="./importmap.json"></script>`))
    
    // 8. 处理应用文件 - 查找并处理所有需要编译的本地文件
    fmt.Println("处理应用文件...")
    
    // 找到所有需要编译的文件
    scriptRegex := regexp.MustCompile(`<script\s+[^>]*src="https://esm\.sh/x"[^>]*href="([^"]+)"[^>]*>`)
    scriptMatches := scriptRegex.FindAllSubmatch(localHTML, -1)
    
    fmt.Printf("发现 %d 个应用入口文件\n", len(scriptMatches))
    
    for _, match := range scriptMatches {
        if len(match) < 2 {
            continue
        }
        
        // 获取相对路径
        relPath := string(match[1])
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
        
        // 替换引用
        replacement := fmt.Sprintf(`<script type="module" src="./%s"></script>`, compiledPath)
        localHTML = scriptRegex.ReplaceAll(localHTML, []byte(replacement))
    }
    
    if err := os.WriteFile(outputIndexPath, localHTML, 0644); err != nil {
        fmt.Printf("保存修改后的 index.html 失败: %v\n", err)
        return fmt.Errorf("保存修改后的 index.html 失败: %v", err)
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
        
        // 如果是 esm.sh 目录，跳过（我们不复制 esm.sh 目录，因为它将由下载过程创建）
        if entry.Name() == "esm.sh" {
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
    
    // 构建自定义 importmap，基于已下载的模块
    customImportMap := make(map[string]string)
    for moduleName, localPath := range globalModuleMap {
        customImportMap[moduleName] = localPath
        
        // 添加常见的子模块映射
        if moduleName == "react" {
            customImportMap["react/jsx-runtime"] = "/esm.sh/react/jsx-runtime"
        } else if moduleName == "react-dom" {
            customImportMap["react-dom/client"] = "/esm.sh/react-dom/client"
        }
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
    }{
        Code:          content,
        Filename:      filename,
        Lang:          lang,
        Target:        "es2022",
        ImportMap:     importMapBytes,
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
    resp, err := http.Post("https://esm.sh/transform", "application/json", strings.NewReader(string(reqBody)))
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
    duplicatePathRegex := regexp.MustCompile(`from\s+["'](/esm\.sh/esm\.sh/([^"']+))["']`)
    compiledCode = duplicatePathRegex.ReplaceAllString(compiledCode, `from "/esm.sh/$2"`)
    
    // 添加路径替换，处理相对路径引用
    // 例如将 "/react-dom@19.1.0/es2022/react-dom.mjs" 替换为 "/esm.sh/react-dom@19.1.0/es2022/react-dom.mjs"
    pathRegex := regexp.MustCompile(`from\s+["'](\/([@\w\d\.-]+)\/[^"']+)["']`)
    compiledCode = pathRegex.ReplaceAllString(compiledCode, `from "/esm.sh$1"`)
    
    // 替换本地相对路径引用的扩展名（.tsx/.ts/.jsx -> .js）
    localImportRegex := regexp.MustCompile(`from\s+["'](\.[^"']+)(\.tsx|\.ts|\.jsx)["']`)
    compiledCode = localImportRegex.ReplaceAllString(compiledCode, `from "$1.js"`)
    
    return compiledCode, nil
}

// 下载子模块
func downloadSubModule(parentModule, subModule, url, outDir string, semaphore chan struct{}, errChan chan error) {
    fmt.Printf("准备下载子模块: %s\n", subModule)
    
    semaphore <- struct{}{}
    defer func() { <-semaphore }()
    
    // 解析URL中的模块路径
    moduleRegex := regexp.MustCompile(`https://esm\.sh/(.+)`)
    matches := moduleRegex.FindStringSubmatch(url)
    
    var modulePath string
    if len(matches) > 1 {
        modulePath = matches[1]
    } else {
        modulePath = subModule
    }
    
    // 使用传入的输出目录
    esmDir := filepath.Join(outDir, "esm.sh")
    wrapperPath := filepath.Join(esmDir, modulePath+".js")
    
    // 创建模块目录
    moduleDir := filepath.Dir(wrapperPath)
    if err := os.MkdirAll(moduleDir, 0755); err != nil {
        fmt.Printf("创建子模块目录失败: %v\n", err)
        errChan <- fmt.Errorf("创建子模块目录失败: %v", err)
        return
    }
    
    // 下载包装器模块
    fmt.Printf("下载子模块: %s\n", url)
    wrapperContent, err := fetchContent(url)
    if err != nil {
        fmt.Printf("下载子模块失败: %v\n", err)
        errChan <- fmt.Errorf("下载子模块失败: %v", err)
        return
    }
    
    // 保存包装器模块
    if err := os.WriteFile(wrapperPath, wrapperContent, 0644); err != nil {
        fmt.Printf("保存子模块失败: %v\n", err)
        errChan <- fmt.Errorf("保存子模块失败: %v", err)
        return
    }
    
    // 从包装器模块中提取实际模块路径
    exportRegex := regexp.MustCompile(`["']([^"']+\.mjs)["']`)
    exportMatches := exportRegex.FindAllSubmatch(wrapperContent, -1)
    
    if len(exportMatches) == 0 {
        fmt.Printf("未在子模块中找到实际模块路径\n")
        globalModuleMap[subModule] = "/" + modulePath + ".js"
        return
    }
    
    // 下载所有实际模块
    for _, match := range exportMatches {
        if len(match) < 2 {
            continue
        }
        
        actualPath := string(match[1])
        if !strings.HasPrefix(actualPath, "/") {
            actualPath = "/" + actualPath
        }
        
        actualUrl := "https://esm.sh" + actualPath
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
        
        // 保存映射
        globalModuleMap[subModule] = actualPath
        break
    }
    
    fmt.Printf("子模块下载成功: %s\n", subModule)
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