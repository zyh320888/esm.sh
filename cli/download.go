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
    
    "github.com/ije/gox/log"
)

// Logger 管理不同类别的日志
type Logger struct {
    logger *log.Logger
    level  string
    categories map[string]bool
}

// 日志类别常量
const (
    LogCatGeneral   = "general"   // 一般日志
    LogCatNetwork   = "network"   // 网络请求日志
    LogCatDependency = "deps"     // 依赖处理日志
    LogCatCompile   = "compile"   // 编译相关日志
    LogCatFS        = "fs"        // 文件系统操作日志
    LogCatContent   = "content"   // 模块内容日志
)

// 全局logger实例
var logger *Logger

// 初始化日志系统
func initLogger(level string, enabledCategories []string) {
    // 使用标准输出
    // 此库只支持 "file:" 协议，我们使用os.Stdout作为输出
    l, err := log.New("file:/dev/stdout")
    
    if err != nil {
        fmt.Printf("初始化日志失败: %v\n", err)
        return
    }
    
    l.SetLevelByName(level)
    
    // 默认启用所有类别
    categories := make(map[string]bool)
    if len(enabledCategories) == 0 {
        categories[LogCatGeneral] = true
        categories[LogCatNetwork] = true
        categories[LogCatDependency] = true
        categories[LogCatCompile] = true
        categories[LogCatFS] = true
        categories[LogCatContent] = true
    } else {
        // 只启用指定的类别
        for _, cat := range enabledCategories {
            categories[cat] = true
        }
    }
    
    logger = &Logger{
        logger: l,
        level: level,
        categories: categories,
    }
}

// 检查类别是否启用
func (l *Logger) isEnabled(category string) bool {
    if l == nil || l.categories == nil {
        return true
    }
    return l.categories[category]
}

// 输出信息级别日志
func (l *Logger) Info(category, format string, v ...interface{}) {
    if l != nil && l.isEnabled(category) {
        l.logger.Infof("[%s] %s", category, fmt.Sprintf(format, v...))
    }
}

// 输出调试级别日志
func (l *Logger) Debug(category, format string, v ...interface{}) {
    if l != nil && l.isEnabled(category) {
        l.logger.Debugf("[%s] %s", category, fmt.Sprintf(format, v...))
    }
}

// 输出错误级别日志
func (l *Logger) Error(category, format string, v ...interface{}) {
    if l != nil && l.isEnabled(category) {
        l.logger.Errorf("[%s] %s", category, fmt.Sprintf(format, v...))
    }
}

// 设置启用的日志类别
func (l *Logger) SetCategories(categories []string) {
    if l == nil {
        return
    }
    
    // 重置所有类别
    l.categories = make(map[string]bool)
    
    // 启用指定的类别
    for _, cat := range categories {
        l.categories[cat] = true
    }
}

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
    // 初始化日志
    initLogger("info", nil) // 默认启用所有类别
    
    logger.Info(LogCatGeneral, "开始执行下载命令...")
    
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
    
    // 日志类别
    logCategories := []string{LogCatGeneral, LogCatNetwork, LogCatDependency, LogCatCompile, LogCatFS, LogCatContent}
    
    logger.Info(LogCatGeneral, "入口路径: %s", entryPath)
    
    // 从参数中获取输出目录和压缩选项
    for i := 1; i < len(args); i++ {
        if args[i] == "--out-dir" && i+1 < len(args) {
            outDir = args[i+1]
            logger.Info(LogCatGeneral, "输出目录: %s", outDir)
            i++
        } else if args[i] == "--minify" {
            minify = true
            logger.Info(LogCatGeneral, "启用代码压缩")
        } else if args[i] == "--api-url" && i+1 < len(args) {
            apiBaseURL = args[i+1]
            logger.Info(LogCatGeneral, "使用API基础URL: %s", apiBaseURL)
            i++
        } else if args[i] == "--deno-json" && i+1 < len(args) {
            denoJsonPath = args[i+1]
            logger.Info(LogCatGeneral, "使用deno.json路径: %s", denoJsonPath)
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
            logger.Info(LogCatGeneral, "使用基础路径: %s", basePath)
            i++
        } else if args[i] == "--log-level" && i+1 < len(args) {
            // 设置日志级别
            initLogger(args[i+1], logCategories)
            logger.Info(LogCatGeneral, "设置日志级别: %s", args[i+1])
            i++
        } else if args[i] == "--log-categories" && i+1 < len(args) {
            // 设置日志类别
            categories := strings.Split(args[i+1], ",")
            logger.SetCategories(categories)
            logger.Info(LogCatGeneral, "启用的日志类别: %s", args[i+1])
            i++
        }
    }

    // 检查入口是文件还是目录
    fileInfo, err := os.Stat(entryPath)
    if err != nil {
        logger.Error(LogCatFS, "获取入口信息失败: %v", err)
        return fmt.Errorf("获取入口信息失败: %v", err)
    }

    // 判断入口文件类型
    var actualEntryPath string
    var indexHtmlPath string
    if fileInfo.IsDir() {
        // 如果是目录，尝试找到 index.html
        logger.Info(LogCatFS, "%s 是目录，查找 index.html...", entryPath)
        indexHtmlPath = filepath.Join(entryPath, "index.html")
        if _, err := os.Stat(indexHtmlPath); err != nil {
            logger.Error(LogCatFS, "在目录 %s 中未找到 index.html: %v", entryPath, err)
            return fmt.Errorf("在目录 %s 中未找到 index.html: %v", entryPath, err)
        }
        logger.Info(LogCatFS, "找到入口文件: %s", indexHtmlPath)
        actualEntryPath = indexHtmlPath
    } else {
        // 直接使用文件
        actualEntryPath = entryPath
    }
    
    // 判断入口文件扩展名
    fileExt := filepath.Ext(actualEntryPath)
    logger.Debug(LogCatFS, "入口文件扩展名: %s", fileExt)
    
    // 检查是否为前端源文件
    isFrontendSource := fileExt == ".tsx" || fileExt == ".ts" || fileExt == ".jsx" || fileExt == ".js"
    
    // 前端源文件需要指定deno.json
    if isFrontendSource && denoJsonPath == "" {
        logger.Error(LogCatGeneral, "入口文件是前端源文件 (%s)，需要同时指定 deno.json 文件", fileExt)
        return fmt.Errorf("入口文件是前端源文件 (%s)，需要同时使用 --deno-json 指定 deno.json 文件", fileExt)
    }
    
    var importMapData struct {
        Imports map[string]string `json:"imports"`
    }
    var entryContent []byte
    
    // 如果指定了deno.json文件路径，从deno.json读取importmap
    if denoJsonPath != "" {
        logger.Info(LogCatFS, "使用指定的deno.json文件: %s", denoJsonPath)
        
        // 读取deno.json文件
        denoJsonContent, err := os.ReadFile(denoJsonPath)
        if err != nil {
            logger.Error(LogCatFS, "读取deno.json文件失败: %v", err)
            return fmt.Errorf("读取deno.json文件失败: %v", err)
        }
        
        // 解析deno.json内容
        if err := json.Unmarshal(denoJsonContent, &importMapData); err != nil {
            logger.Error(LogCatGeneral, "解析deno.json内容失败: %v", err)
            return fmt.Errorf("解析deno.json内容失败: %v", err)
        }
        
        if importMapData.Imports == nil {
            logger.Error(LogCatGeneral, "deno.json不包含有效的imports字段")
            return fmt.Errorf("deno.json不包含有效的imports字段")
        }
        
        logger.Debug(LogCatDependency, "从deno.json解析到的importmap: %v", importMapData.Imports)
        
        // 自动添加常用的React相关子模块
        addReactJsxRuntime(&importMapData)
    } else {
        // 从HTML中解析importmap
        // 如果是HTML文件，从中解析importmap
        logger.Info(LogCatGeneral, "入口文件是HTML文件，从中解析importmap")
        
        // 读取入口文件
        logger.Debug(LogCatFS, "正在读取入口文件: %s", actualEntryPath)
        entryContent, err = os.ReadFile(actualEntryPath)
        if err != nil {
            logger.Error(LogCatFS, "读取入口文件失败: %v", err)
            return fmt.Errorf("读取入口文件失败: %v", err)
        }
        logger.Debug(LogCatFS, "入口文件读取成功")
        
        // 解析 importmap
        logger.Info(LogCatDependency, "正在解析 importmap...")
        
        // 使用正则表达式从 HTML 中提取 importmap
        importMapRegex := regexp.MustCompile(`<script\s+type="importmap"\s*>([\s\S]*?)<\/script>`)
        matches := importMapRegex.FindSubmatch(entryContent)
        
        if len(matches) < 2 {
            logger.Error(LogCatDependency, "未在入口文件中找到 importmap")
            return fmt.Errorf("未在入口文件中找到 importmap")
        }
        
        importMapJson := matches[1]
        logger.Debug(LogCatDependency, "找到 importmap: %s", string(importMapJson))
        
        if err := json.Unmarshal(importMapJson, &importMapData); err != nil {
            logger.Error(LogCatDependency, "解析 importmap 失败: %v", err)
            return fmt.Errorf("解析 importmap 失败: %v", err)
        }
        
        if importMapData.Imports == nil {
            logger.Error(LogCatDependency, "importmap 不包含有效的 imports 字段")
            return fmt.Errorf("importmap 不包含有效的 imports 字段")
        }
        
        logger.Debug(LogCatDependency, "解析到的 importmap: %v", importMapData.Imports)
        
        // 自动添加常用的React相关子模块
        addReactJsxRuntime(&importMapData)
    }

    // 3. 创建输出目录
    logger.Info(LogCatFS, "正在创建输出目录: %s", outDir)
    if err := os.MkdirAll(outDir, 0755); err != nil {
        logger.Error(LogCatFS, "创建输出目录失败: %v", err)
        return fmt.Errorf("创建输出目录失败: %v", err)
    }
    
    // 从API URL中提取域名部分作为目录名
    apiDomain := getAPIDomain()
    
    // 创建目录
    esmDir := filepath.Join(outDir, apiDomain)
    if err := os.MkdirAll(esmDir, 0755); err != nil {
        logger.Error(LogCatFS, "创建 %s 目录失败: %v", apiDomain, err)
        return fmt.Errorf("创建 %s 目录失败: %v", apiDomain, err)
    }

    // 4. 使用并发下载所有依赖
    logger.Info(LogCatDependency, "开始下载依赖，共 %d 个", len(importMapData.Imports))
    var wg sync.WaitGroup
    errChan := make(chan error, len(importMapData.Imports))
    semaphore := make(chan struct{}, 5) // 限制并发数
    
    // 保存模块URL和本地路径的映射
    moduleMap := make(map[string]string)

    // 下载所有依赖
    for spec, url := range importMapData.Imports {
        logger.Debug(LogCatDependency, "准备下载依赖: %s -> %s", spec, url)
        wg.Add(1)
        go downloadAndProcessModule(spec, url, outDir, &wg, semaphore, errChan, moduleMap)
    }

    // 等待所有下载完成
    logger.Info(LogCatDependency, "等待所有下载完成...")
    wg.Wait()
    close(errChan)

    // 收集错误
    var errors []string
    for err := range errChan {
        errors = append(errors, err.Error())
    }

    if len(errors) > 0 {
        logger.Error(LogCatGeneral, "下载过程中出现错误:")
        for _, err := range errors {
            logger.Error(LogCatGeneral, "%s", err)
        }
        return fmt.Errorf("下载过程中出现错误:\n%s", strings.Join(errors, "\n"))
    }

    // 5. 复制项目文件到输出目录
    if fileInfo.IsDir() {
        // 如果入口是目录，需要复制整个目录
        logger.Info(LogCatFS, "正在复制项目文件到输出目录...")
        err = copyDir(entryPath, outDir)
        if err != nil {
            logger.Error(LogCatFS, "复制项目文件失败: %v", err)
            return fmt.Errorf("复制项目文件失败: %v", err)
        }
    } else {
        // 检查是否为前端源文件
        if isFrontendSource {
            // 如果是前端源文件，直接编译该文件
            logger.Info(LogCatCompile, "入口文件是前端源文件，直接编译处理: %s", actualEntryPath)
            
            // 获取源文件的相对路径
            relPath := filepath.Base(actualEntryPath)
            
            // 编译应用文件
            if err := compileAppFilesWithPath(actualEntryPath, relPath, outDir); err != nil {
                logger.Error(LogCatCompile, "编译前端源文件失败: %v", err)
                return fmt.Errorf("编译前端源文件失败: %v", err)
            }
            
            logger.Info(LogCatCompile, "前端源文件编译完成: %s", actualEntryPath)
        } else {
            // 如果是单个HTML文件，复制这个文件
            logger.Info(LogCatFS, "正在复制入口文件到输出目录: %s", entryPath)
            targetPath := filepath.Join(outDir, filepath.Base(entryPath))
            if err := os.WriteFile(targetPath, entryContent, 0644); err != nil {
                logger.Error(LogCatFS, "保存入口文件失败: %v", err)
                return fmt.Errorf("保存入口文件失败: %v", err)
            }
        }
    }

    // 6. 生成本地 importmap
    logger.Info(LogCatDependency, "生成本地 importmap...")
    
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
        logger.Error(LogCatDependency, "生成本地 importmap 失败: %v", err)
        return fmt.Errorf("生成本地 importmap 失败: %v", err)
    }
    
    if err := os.WriteFile(filepath.Join(outDir, "importmap.json"), importMapContent, 0644); err != nil {
        logger.Error(LogCatFS, "保存本地 importmap 失败: %v", err)
        return fmt.Errorf("保存本地 importmap 失败: %v", err)
    }
    
    // 7. 修改输出目录中的 index.html (如果存在)
    outputIndexPath := filepath.Join(outDir, "index.html")
    if _, err := os.Stat(outputIndexPath); err == nil && !isFrontendSource {
        logger.Info(LogCatFS, "修改输出目录中的 index.html...")
        
        // 读取输出目录中的 index.html
        outputIndexContent, err := os.ReadFile(outputIndexPath)
        if err != nil {
            logger.Error(LogCatFS, "读取输出目录中的 index.html 失败: %v", err)
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
        logger.Info(LogCatCompile, "处理应用文件...")
        
        // 找到所有需要编译的文件
        scriptRegex := regexp.MustCompile(`<script\s+[^>]*src="https://esm\.(sh|d8d\.fun)/x"[^>]*href="([^"]+)"[^>]*>(?:</script>)?`)
        scriptMatches := scriptRegex.FindAllSubmatch(localHTML, -1)
        
        logger.Info(LogCatCompile, "发现 %d 个应用入口文件", len(scriptMatches))
        
        for _, match := range scriptMatches {
            if len(match) < 3 {
                continue
            }
            
            // 获取相对路径
            relPath := string(match[2])
            logger.Debug(LogCatCompile, "发现入口文件: %s", relPath)
            
            // 使用入口的完整路径
            fullPath := filepath.Join(filepath.Dir(indexHtmlPath), relPath)
            logger.Debug(LogCatCompile, "使用源文件的完整路径: %s", fullPath)
            
            // 编译前检查路径
            if _, err := os.Stat(fullPath); os.IsNotExist(err) {
                logger.Error(LogCatFS, "警告: 源文件不存在: %s", fullPath)
                return fmt.Errorf("源文件不存在: %s", fullPath)
            }
            
            // 修改compileAppFiles调用，传入入口文件的完整路径和相对路径
            err = compileAppFilesWithPath(fullPath, relPath, outDir)
            if err != nil {
                logger.Error(LogCatCompile, "编译应用文件失败: %v", err)
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
            logger.Error(LogCatFS, "保存修改后的 index.html 失败: %v", err)
            return fmt.Errorf("保存修改后的 index.html 失败: %v", err)
        }
    }

    logger.Info(LogCatGeneral, "下载完成！所有文件已保存到 %s 目录", outDir)
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
    logger.Debug(LogCatNetwork, "发送HTTP请求: %s", url)
    resp, err := client.Get(url)
    if err != nil {
        logger.Error(LogCatNetwork, "HTTP请求失败: %v", err)
        return nil, err
    }
    defer resp.Body.Close()
    
    // 处理重定向
    if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusMovedPermanently || 
       resp.StatusCode == http.StatusTemporaryRedirect || resp.StatusCode == http.StatusPermanentRedirect {
        redirectURL, err := resp.Location()
        if err != nil {
            logger.Error(LogCatNetwork, "获取重定向URL失败: %v", err)
            return nil, fmt.Errorf("获取重定向URL失败: %v", err)
        }
        logger.Debug(LogCatNetwork, "发现重定向: %s -> %s", url, redirectURL.String())
        return fetchContent(redirectURL.String())
    }
    
    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        logger.Error(LogCatNetwork, "HTTP 错误: %d %s - %s", resp.StatusCode, resp.Status, string(body))
        return nil, fmt.Errorf("HTTP 错误: %d %s - %s", resp.StatusCode, resp.Status, string(body))
    }
    
    content, err := io.ReadAll(resp.Body)
    if err != nil {
        logger.Error(LogCatNetwork, "读取响应内容失败: %v", err)
        return nil, err
    }
    
    logger.Debug(LogCatNetwork, "成功获取内容，大小: %d 字节", len(content))
    return content, nil
}

// 复制目录
func copyDir(src, dst string) error {
    // 获取源目录信息
    srcInfo, err := os.Stat(src)
    if err != nil {
        logger.Error(LogCatFS, "获取源目录信息失败: %v", err)
        return err
    }
    
    // 创建目标目录
    if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
        logger.Error(LogCatFS, "创建目标目录失败: %v", err)
        return err
    }
    
    // 读取源目录内容
    entries, err := os.ReadDir(src)
    if err != nil {
        logger.Error(LogCatFS, "读取源目录内容失败: %v", err)
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
            logger.Debug(LogCatFS, "跳过API目录: %s", entry.Name())
            continue
        }
        
        // 跳过 TypeScript 和 JSX 源文件，这些文件会被编译
        if !entry.IsDir() {
            ext := filepath.Ext(entry.Name())
            if ext == ".tsx" || ext == ".ts" || ext == ".jsx" {
                logger.Debug(LogCatFS, "跳过源文件: %s", srcPath)
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
    logger.Debug(LogCatFS, "复制文件: %s -> %s", src, dst)
    
    // 打开源文件
    srcFile, err := os.Open(src)
    if err != nil {
        logger.Error(LogCatFS, "打开源文件失败: %v", err)
        return err
    }
    defer srcFile.Close()
    
    // 创建目标文件
    dstFile, err := os.Create(dst)
    if err != nil {
        logger.Error(LogCatFS, "创建目标文件失败: %v", err)
        return err
    }
    defer dstFile.Close()
    
    // 复制内容
    _, err = io.Copy(dstFile, srcFile)
    if err != nil {
        logger.Error(LogCatFS, "复制文件内容失败: %v", err)
        return err
    }
    
    // 获取源文件权限
    srcInfo, err := os.Stat(src)
    if err != nil {
        logger.Error(LogCatFS, "获取源文件信息失败: %v", err)
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
        logger.Debug(LogCatCompile, "CSS文件不需要编译: %s", filename)
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
        logger.Error(LogCatCompile, "不支持的文件类型: %s", fileExt)
        return "", fmt.Errorf("不支持的文件类型: %s", fileExt)
    }
    
    logger.Debug(LogCatCompile, "编译文件 %s，类型: %s", filename, lang)
    
    // 提取域名部分，用于后续处理
    apiDomain := strings.TrimPrefix(strings.TrimPrefix(apiBaseURL, "https://"), "http://")
    
    // 构建自定义 importmap，基于已下载的模块
    customImportMap := make(map[string]string)
    for moduleName, localPath := range globalModuleMap {
        customImportMap[moduleName] = localPath
    }
    
    importMapBytes, err := json.Marshal(map[string]map[string]string{
        "imports": customImportMap,
    })
    if err != nil {
        logger.Error(LogCatCompile, "创建 importmap 失败: %v", err)
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
        logger.Error(LogCatCompile, "序列化请求失败: %v", err)
        return "", fmt.Errorf("序列化请求失败: %v", err)
    }
    
    // 发送请求
    logger.Debug(LogCatNetwork, "发送编译请求: %s/transform", apiBaseURL)
    resp, err := http.Post(apiBaseURL + "/transform", "application/json", strings.NewReader(string(reqBody)))
    if err != nil {
        logger.Error(LogCatNetwork, "发送编译请求失败: %v", err)
        return "", fmt.Errorf("发送请求失败: %v", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        logger.Error(LogCatNetwork, "编译请求失败: %d %s - %s", resp.StatusCode, resp.Status, string(body))
        return "", fmt.Errorf("请求失败: %d %s - %s", resp.StatusCode, resp.Status, string(body))
    }
    
    // 解析响应
    var result struct {
        Code string `json:"code"`
        Map  string `json:"map"`
    }
    
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        logger.Error(LogCatCompile, "解析编译响应失败: %v", err)
        return "", fmt.Errorf("解析响应失败: %v", err)
    }
    
    logger.Debug(LogCatCompile, "编译成功，处理编译后代码")
    
    // 进一步处理编译后的代码，将引用替换为本地路径
    compiledCode := result.Code
    
    // 使用processWrapperContent处理绝对路径导入
    processedCode := processWrapperContent([]byte(compiledCode), apiDomain)
    compiledCode = string(processedCode)
    
    // 替换本地相对路径引用的扩展名（.tsx/.ts/.jsx -> .js）
    localImportRegex := regexp.MustCompile(`from\s+["'](\.[^"']+)(\.tsx|\.ts|\.jsx)["']`)
    compiledCode = localImportRegex.ReplaceAllString(compiledCode, `from "$1.js"`)
    
    logger.Debug(LogCatCompile, "编译文件处理完成: %s", filename)
    return compiledCode, nil
}

// 规范化模块路径，处理扩展名和index.js
func normalizeModulePath(path string) string {
    // 分离路径和查询参数
    var query string
    if strings.Contains(path, "?") {
        pathParts := strings.SplitN(path, "?", 2)
        path = pathParts[0]
        query = "?" + pathParts[1]
    } else {
        query = ""
    }
    
    // 处理路径部分
    pathParts := strings.Split(path, "/")
    
    // 获取路径的各部分
    lastIndex := len(pathParts) - 1
    lastPart := ""
    if lastIndex >= 0 {
        lastPart = pathParts[lastIndex]
    }
    
    // 检查是否为作用域包（@开头的包）
    isScope := false
    scopeIndex := -1
    for i, part := range pathParts {
        if part != "" && strings.HasPrefix(part, "@") {
            isScope = true
            scopeIndex = i
            break
        }
    }
    
    // 检查是否是包的主入口模块（不包含子路径部分的包引用）
    isMainModule := false
    
    // 对于像 "react@19.0.0" 或 "antd@5.24.5" 这样直接引用包而没有子路径的情况，应视为主模块
    if !isScope && len(pathParts) <= 2 {
        // 检查最后一部分是否包含版本号 @x.y.z
        if lastPart != "" && strings.Contains(lastPart, "@") && !strings.HasPrefix(lastPart, "@") {
            isMainModule = true
        }
    }
    
    // 根据路径类型添加适当的后缀
    if isScope {
        // 检查是否是作用域包的主模块
        // 主模块判断: 后面没有更多路径部分，或者直接是@xxxx/yyyy格式
        isScopeMainModule := false
        
        // 如果作用域包名后面没有更多路径部分（@scope/pkg 或 @scope/pkg/）
        if scopeIndex < len(pathParts)-2 {
            // 有超过包名以外的路径，是子模块
            isScopeMainModule = false
        } else if scopeIndex == len(pathParts)-2 {
            // 刚好是包名（@scope/pkg），是主模块
            isScopeMainModule = true
        } else if scopeIndex == len(pathParts)-1 && strings.Contains(pathParts[scopeIndex], "/") {
            // 如果@scope/pkg被当作一个整体在路径中，也视为主模块
            isScopeMainModule = true
        }
        
        if isScopeMainModule || lastPart == "" || strings.HasSuffix(path, "/") {
            // 作用域包主模块，添加index.js
            // 例如 /@ant-design/icons 或 /@ant-design/icons/ -> /@ant-design/icons/index.js
            if strings.HasSuffix(path, "/") {
                path = path + "index.js"
            } else {
                path = path + "/index.js"
            }
            logger.Debug(LogCatDependency, "为作用域包主模块添加index.js: %s", path)
        } else if !strings.HasSuffix(lastPart, ".js") && !strings.HasSuffix(lastPart, ".mjs") && !strings.HasSuffix(lastPart, ".cjs") {
            // 作用域包子模块，添加.js后缀
            // 例如 /@ant-design/icons/xxx -> /@ant-design/icons/xxx.js
            pathParts[len(pathParts)-1] = lastPart + ".js"
            path = strings.Join(pathParts, "/")
            logger.Debug(LogCatDependency, "为作用域包子模块添加.js后缀: %s", path)
        }
    } else if isMainModule || lastPart == "" || !strings.Contains(path, "/") || strings.HasSuffix(path, "/") {
        // 普通主模块，添加index.js
        // 例如 /react-dom@19.0.0 或 /react-dom@19.0.0/ -> /react-dom@19.0.0/index.js
        if !strings.HasSuffix(path, "/index.js") && !strings.HasSuffix(path, "/index.mjs") {
            if strings.HasSuffix(path, "/") {
                path = path + "index.js"
            } else {
                path = path + "/index.js"
            }
            logger.Debug(LogCatDependency, "为普通主模块添加index.js: %s", path)
        }
    } else if !strings.HasSuffix(lastPart, ".js") && !strings.HasSuffix(lastPart, ".mjs") && !strings.HasSuffix(lastPart, ".cjs") {
        // 普通子模块，添加.js后缀
        // 例如 /react-dom@19.0.0/utils -> /react-dom@19.0.0/utils.js
        pathParts[len(pathParts)-1] = lastPart + ".js"
        path = strings.Join(pathParts, "/")
        logger.Debug(LogCatDependency, "为普通子模块添加.js后缀: %s", path)
    }
    
    // 重新添加查询参数
    return path + query
}

// 下载并处理模块的通用函数
func downloadAndProcessModule(spec, url, outDir string, wg *sync.WaitGroup, semaphore chan struct{}, errChan chan error, localModuleMap map[string]string) {
    // 如果提供了waitgroup，在完成时通知
    if wg != nil {
        defer wg.Done()
    }
    
    // 如果提供了信号量，获取许可
    if semaphore != nil {
        semaphore <- struct{}{}
        defer func() { <-semaphore }()
    }

    logger.Debug(LogCatDependency, "开始处理模块: %s", url)
    
    // 检查是否已下载过此模块
    downloadedModulesMutex.Lock()
    alreadyDownloaded := downloadedModules[url]
    downloadedModulesMutex.Unlock()
    if alreadyDownloaded {
        logger.Debug(LogCatDependency, "模块已下载过，跳过: %s", url)
        return
    }
    
    // 标记该URL已经处理过
    downloadedModulesMutex.Lock()
    downloadedModules[url] = true
    downloadedModulesMutex.Unlock()
    
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
    
    logger.Debug(LogCatDependency, "从URL中提取的模块路径: %s", modulePath)
    
    // 提取域名部分，用于后续处理
    apiDomain := getAPIDomain()
    
    // 使用传入的输出目录和API域名
    esmDir := filepath.Join(outDir, apiDomain)
    
    // 使用normalizeModulePath规范化路径
    normalizedPath := normalizeModulePath("/" + modulePath)
    // 移除前导斜杠，因为filepath.Join不需要它
    normalizedPath = strings.TrimPrefix(normalizedPath, "/")
    
    // 确定模块的保存路径
    moduleSavePath := filepath.Join(esmDir, normalizedPath)
    
    // 创建模块目录
    if err := os.MkdirAll(filepath.Dir(moduleSavePath), 0755); err != nil {
        logger.Error(LogCatFS, "创建模块目录失败: %v", err)
        if errChan != nil {
            errChan <- fmt.Errorf("创建模块目录失败: %v", err)
        }
        return
    }
    
    // 下载模块内容
    logger.Info(LogCatNetwork, "下载模块: %s，保存到: %s", url, moduleSavePath)
    moduleContent, err := fetchContent(url)
    if err != nil {
        logger.Error(LogCatNetwork, "下载模块失败: %v", err)
        if errChan != nil {
            errChan <- fmt.Errorf("下载模块失败: %v", err)
        }
        return
    }
    
    // 处理模块内容中的路径
    processedContent := processWrapperContent(moduleContent, apiDomain)
    
    // 定义头N字节变量
    headN := 200
    // 仅在处理前后内容一样时才显示日志
    if string(moduleContent) == string(processedContent) {
        logger.Debug(LogCatContent, "处理模块内容中的依赖路径: %s", url)
        // logger.Debug(LogCatContent, "检测到内容未发生变化")
        // 显示处理前的内容头100字节（仅调试级别）
        if len(moduleContent) > headN {
            logger.Debug(LogCatContent, "处理前的内容头 %d 字节: %s", headN, string(moduleContent[:headN]))
        } else {
            logger.Debug(LogCatContent, "处理前的内容: %s", string(moduleContent))
        }
        // logger.Debug(LogCatContent, "处理模块内容中的依赖路径完成: %s", url)
    } else {
        // logger.Debug(LogCatContent, "内容已发生变化")
    }
    
    // 保存处理后的模块
    if err := os.WriteFile(moduleSavePath, processedContent, 0644); err != nil {
        logger.Error(LogCatFS, "保存模块失败: %v", err)
        if errChan != nil {
            errChan <- fmt.Errorf("保存模块失败: %v", err)
        }
        return
    }
    
    // 查找模块中的深层依赖（在处理内容之前）
    depPaths := findDeepDependencies(moduleContent, normalizedPath)
    logger.Debug(LogCatDependency, "分析模块中的依赖: %s", url)
    if len(depPaths) > 0 {
        logger.Info(LogCatDependency, "✅ 共发现 %d 个依赖", len(depPaths))
    } else {
        logger.Debug(LogCatDependency, "⚠️ 未发现任何依赖")
    }
    
    
    // 设置模块映射（如果提供了spec）
    if spec != "" {
        // 检查modulePath是否有扩展名
        ext := filepath.Ext(modulePath)
        // 如果是子模块使用完整路径
        if strings.Contains(spec, "/") {
            if ext == "" || (ext != ".js" && ext != ".mjs" && ext != ".cjs") {
                // 没有扩展名，添加.js
                if localModuleMap != nil {
                    localModuleMap[spec] = "/" + modulePath + ".js"
                }
                globalModuleMap[spec] = "/" + modulePath + ".js"
            } else {
                // 已有扩展名，不添加.js
                if localModuleMap != nil {
                    localModuleMap[spec] = "/" + modulePath
                }
                globalModuleMap[spec] = "/" + modulePath
            }
        } else {
            // 主模块使用index.js
            if localModuleMap != nil {
                localModuleMap[spec] = "/" + modulePath + "/index.js"
            }
            globalModuleMap[spec] = "/" + modulePath + "/index.js"
        }
    } else if modulePath != "" {
        // 对于子模块，也添加到全局映射中
        ext := filepath.Ext(modulePath)
        if ext == "" || (ext != ".js" && ext != ".mjs" && ext != ".cjs") {
            // 没有扩展名，添加.js
            globalModuleMap[modulePath] = "/" + modulePath + ".js"
        } else {
            // 已有扩展名，不添加.js
            globalModuleMap[modulePath] = "/" + modulePath
        }
    }
    
    // 下载所有依赖
    for _, depPath := range depPaths {
        depUrl := apiBaseURL + depPath
        downloadedModulesMutex.Lock()
        alreadyDownloaded := downloadedModules[depUrl]
        downloadedModulesMutex.Unlock()
        if !alreadyDownloaded {
            logger.Info(LogCatDependency, "🚀 开始递归下载依赖: %s", depUrl)
            if wg != nil {
                wg.Add(1)
            }
            go downloadAndProcessModule("", depUrl, outDir, wg, semaphore, errChan, localModuleMap)
        } else {
            logger.Debug(LogCatDependency, "⏩ 跳过已下载的依赖: %s", depUrl)
        }
    }
    
    // // 查找裸导入
    // bareImports := findBareImports(moduleContent)
    // for _, imp := range bareImports {
    //     if !isLocalPath(imp) && !strings.HasPrefix(imp, "/") {
    //         depURL := constructDependencyURL(imp, apiBaseURL)
    //         downloadedModulesMutex.Lock()
    //         alreadyDownloaded := downloadedModules[depURL]
    //         downloadedModulesMutex.Unlock()
    //         if depURL != "" && !alreadyDownloaded {
    //             logger.Info(LogCatDependency, "📦 递归下载裸依赖: %s -> %s", imp, depURL)
    //             if wg != nil {
    //                 wg.Add(1)
    //             }
    //             go downloadAndProcessModule("", depURL, outDir, wg, semaphore, errChan, localModuleMap)
    //         } else if depURL != "" {
    //             logger.Debug(LogCatDependency, "⏩ 跳过已下载的裸依赖: %s", depURL)
    //         }
    //     }
    // }
    
    logger.Debug(LogCatDependency, "模块处理完成: %s", url)
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
    
    logger.Debug(LogCatFS, "源文件根目录: %s", baseDir)
    
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
            logger.Debug(LogCatFS, "使用入口文件的完整路径: %s", srcPath)
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
            
            logger.Debug(LogCatFS, "计算依赖文件路径: %s", srcPath)
        }
        
        // 检查文件是否存在
        if _, err := os.Stat(srcPath); os.IsNotExist(err) {
            // 尝试其他可能的路径
            cleanCurrentFile := strings.TrimPrefix(currentFile, "./")
            altPath := filepath.Join(filepath.Dir(baseDir), cleanCurrentFile)
            if _, err := os.Stat(altPath); err == nil {
                srcPath = altPath
                logger.Debug(LogCatFS, "使用替代路径: %s", srcPath)
            } else {
                return fmt.Errorf("找不到源文件: %s", srcPath)
            }
        }
        
        // 编译后的文件保存在输出目录
        outputPath := filepath.Join(outDir, strings.TrimSuffix(currentFile, filepath.Ext(currentFile)) + ".js")
        logger.Debug(LogCatFS, "编译文件: %s -> %s", srcPath, outputPath)
        
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
            logger.Debug(LogCatFS, "复制非模块文件: %s -> %s", srcPath, filepath.Join(outDir, currentFile))
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
            logger.Debug(LogCatFS, "发现本地依赖: 从 %s 导入 %s -> 解析为 %s", importDir, imp, resolvedPath)
            
            // 优先检查当前目录的相对路径
            relativeToCurrentFile := filepath.Join(filepath.Dir(srcPath), strings.TrimPrefix(imp, "./"))
            if _, err := os.Stat(relativeToCurrentFile); err == nil {
                resolvedPath = filepath.Clean(filepath.Join(filepath.Dir(currentFile), strings.TrimPrefix(imp, "./")))
                logger.Debug(LogCatFS, "使用相对当前文件的路径: %s", resolvedPath)
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
            logger.Debug(LogCatFS, "原始导入路径: %s", importPath)
            
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
        logger.Debug(LogCatFS, "未找到%s模块，不添加%s/%s子模块", baseModule, baseModule, subModule)
        return
    }
    
    // 子模块完整名称
    fullSubModuleName := baseModule + "/" + subModule
    
    // 检查是否已经包含子模块
    if _, exists := data.Imports[fullSubModuleName]; !exists {
        logger.Debug(LogCatFS, "自动添加%s子模块", fullSubModuleName)
        
        // 从基础URL中提取版本信息
        versionRegex := regexp.MustCompile(baseModule + `@([\d\.]+)`)
        matches := versionRegex.FindStringSubmatch(baseUrl)
        
        var version string
        if len(matches) > 1 {
            version = matches[1]
            logger.Debug(LogCatFS, "检测到%s版本: %s", baseModule, version)
            
            // 根据版本构造子模块URL
            subModuleUrl := strings.Replace(baseUrl, baseModule+"@"+version, baseModule+"@"+version+"/"+subModule, 1)
            data.Imports[fullSubModuleName] = subModuleUrl
            logger.Debug(LogCatFS, "添加%s模块: %s", fullSubModuleName, subModuleUrl)
        } else {
            // 如果无法确定版本，使用与基础模块相同的URL结构
            logger.Debug(LogCatFS, "无法从URL确定%s版本，使用与%s相同的URL结构", baseModule, baseModule)
            
            // 构造子模块URL，替换路径部分
            subModuleUrl := strings.Replace(baseUrl, baseModule, baseModule+"/"+subModule, 1)
            data.Imports[fullSubModuleName] = subModuleUrl
            logger.Debug(LogCatFS, "添加%s模块: %s", fullSubModuleName, subModuleUrl)
        }
    }
}

// 处理包装器模块的内容，修正其中的导入路径
func processWrapperContent(content []byte, apiDomain string) []byte {
    contentStr := string(content)

    // 处理裸导入路径，添加API域名前缀
    // 如 import "/react-dom@19.0.0/es2022/react-dom.mjs" 
    // import*as __0$ from"/react@19.0.0/es2022/react.mjs";
    // 变为 import "/esm.d8d.fun/react-dom@19.0.0/es2022/react-dom.mjs"
    importRegex := regexp.MustCompile(`(?:import\s*\*?\s*as\s*[^"']*\s*from|import\s*\{[^}]*\}\s*from|import|export\s*\*\s*from|export\s*\{\s*[^}]*\}\s*from)\s*["'](\/.+?)["']`)
    contentStr = importRegex.ReplaceAllStringFunc(contentStr, func(match string) string {
        parts := importRegex.FindStringSubmatch(match)
        if len(parts) >= 2 {
            originalPath := parts[1]
            
            // 检查路径是否已经包含API域名
            if !strings.Contains(originalPath, "/"+apiDomain+"/") {
                // 规范化模块路径
                normalizedPath := normalizeModulePath(originalPath)
                
                // 替换为带API域名的路径
                var newPath string
                if basePath != "" && !strings.HasPrefix(normalizedPath, basePath) {
                    // 如果设置了basePath，添加前缀
                    newPath = basePath + "/" + apiDomain + normalizedPath
                } else {
                    newPath = "/" + apiDomain + normalizedPath
                }
                
                return strings.Replace(match, originalPath, newPath, 1)
            }
        }
        return match
    })

    return []byte(contentStr)
}

// 从模块内容中找出深层依赖
func findDeepDependencies(content []byte, currentModulePath string) []string {
    // 提取形如 "/react-dom@19.0.0/es2022/react-dom.mjs" 的依赖路径
    // import*as __0$ from"/react@19.0.0/es2022/react.mjs";
    dependencyRegex := regexp.MustCompile(`(?:import\s*\*?\s*as\s*[^"']*\s*from|import\s*\{[^}]*\}\s*from|import\s+[^"'\s]+\s+from|import|export\s*\*\s*from|export\s*\{\s*[^}]*\}\s*from)\s*["']((?:\/|\.[\.\/]).*?)["']`)
    matches := dependencyRegex.FindAllSubmatch(content, -1)
    
    var deps []string
    seen := make(map[string]bool)
    
    // 添加日志：显示正在分析的内容长度
    logger.Debug(LogCatDependency, "正在分析模块内容，长度: %d 字节", len(content))
    logger.Debug(LogCatDependency, "当前模块路径: %s", currentModulePath)
    
    // 当前模块的目录路径，用于解析相对路径
    var currentDir string
    if currentModulePath != "" {
        // 去掉文件名部分，只保留目录
        currentDir = filepath.Dir(currentModulePath)
        if !strings.HasSuffix(currentDir, "/") {
            currentDir = currentDir + "/"
        }
        logger.Debug(LogCatDependency, "当前模块目录: %s", currentDir)
    }
    
    for _, match := range matches {
        if len(match) >= 2 {
            depPath := string(match[1])
            
            // 处理可能的查询参数
            var queryParams string
            if strings.Contains(depPath, "?") {
                pathParts := strings.SplitN(depPath, "?", 2)
                depPath = pathParts[0] // 只使用问号前的路径部分
                queryParams = "?" + pathParts[1]
                logger.Debug(LogCatDependency, "路径包含查询参数，提取基本路径: %s, 查询参数: %s", depPath, queryParams)
            }
            
            // 处理相对路径
            if strings.HasPrefix(depPath, "./") || strings.HasPrefix(depPath, "../") {
                if currentDir != "" {
                    // 解析相对路径为绝对路径
                    var resolvedPath string
                    
                    // 简单处理 "./" 开头的路径
                    if strings.HasPrefix(depPath, "./") {
                        resolvedPath = currentDir + strings.TrimPrefix(depPath, "./")
                    } else if strings.HasPrefix(depPath, "../") {
                        // 处理 "../" 开头的路径（可能有多层）
                        parts := strings.Split(currentDir, "/")
                        // 移除最后一个空元素（如果有）
                        if len(parts) > 0 && parts[len(parts)-1] == "" {
                            parts = parts[:len(parts)-1]
                        }
                        
                        relPath := depPath
                        
                        // 处理每个 "../"
                        for strings.HasPrefix(relPath, "../") {
                            if len(parts) > 1 {
                                // 移除最后一个目录部分
                                parts = parts[:len(parts)-1]
                                relPath = strings.TrimPrefix(relPath, "../")
                            } else {
                                logger.Error(LogCatDependency, "相对路径超出根目录范围: %s", depPath)
                                break
                            }
                        }
                        
                        // 构建新路径
                        base := strings.Join(parts, "/")
                        if base != "" && !strings.HasSuffix(base, "/") {
                            base = base + "/"
                        }
                        resolvedPath = base + relPath
                    }
                    
                    logger.Debug(LogCatDependency, "解析相对路径: %s -> %s", depPath, resolvedPath)
                    depPath = resolvedPath
                } else {
                    logger.Error(LogCatDependency, "无法解析相对路径，当前模块路径为空: %s", depPath)
                    continue
                }
            }
            
            // 确保路径以斜杠开头
            if !strings.HasPrefix(depPath, "/") {
                depPath = "/" + depPath
            }
            
            // 重新添加查询参数
            if queryParams != "" {
                depPath = depPath + queryParams
            }
            
            if !seen[depPath] {
                seen[depPath] = true
                deps = append(deps, depPath)
                // 添加日志：每发现一个依赖就记录
                logger.Debug(LogCatDependency, "🔍 发现依赖: %s", depPath)
            }
        }
    }
    
    return deps
}

// PrintLogHelp 输出日志分类帮助信息
func PrintLogHelp() {
    fmt.Println("日志分类系统帮助：")
    fmt.Println("  --log-level 选项：设置日志级别 (debug, info, warn, error)")
    fmt.Println("  --log-categories 选项：设置启用的日志类别，用逗号分隔")
    fmt.Println()
    fmt.Println("可用的日志类别：")
    fmt.Printf("  %s: 一般性日志信息\n", LogCatGeneral)
    fmt.Printf("  %s: 网络请求相关日志\n", LogCatNetwork)
    fmt.Printf("  %s: 依赖分析和处理日志\n", LogCatDependency)
    fmt.Printf("  %s: 编译相关日志\n", LogCatCompile)
    fmt.Printf("  %s: 文件系统操作日志\n", LogCatFS)
    fmt.Printf("  %s: 模块内容显示日志\n", LogCatContent)
    fmt.Println()
    fmt.Println("示例:")
    fmt.Println("  esm download ./project --log-level=debug --log-categories=general,network")
    fmt.Println("  esm download ./app.tsx --deno-json=deno.json --log-categories=compile,deps")
} 