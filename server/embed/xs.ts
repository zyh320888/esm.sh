/*!
 * 🔄 esm.sh/xs - 直接从URL加载并编译 tsx/jsx/ts 文件
 * 使用方法: <script src="https://esm.sh/xs" href="https://example.com/app.tsx">
 * 
 * 支持的属性:
 * - href: 要加载的脚本URL (必需)
 * - version: 版本号，用于避免缓存 (可选)
 * - no-cache: 存在时禁用缓存 (可选)
 * - refresh: 存在时强制刷新缓存 (可选)
 * - check-modified: 存在时检查文件是否被修改 (可选)
 * - max-age: 缓存最大有效期，单位秒，默认3600 (1小时) (可选)
 * - credentials: 存在时使用凭据请求资源 (可选)
 */

const d = document;
const l = localStorage;
const stringify = JSON.stringify;
const hostname = location.hostname;

async function run() {
  const currentScript = d.currentScript as HTMLScriptElement | null;
  if (!currentScript) {
    console.error("[esm.sh/xs] 无法获取当前脚本元素");
    return;
  }
  
  // 获取 href 属性值
  const href = currentScript.getAttribute("href");
  if (!href) {
    console.error("[esm.sh/xs] 缺少 href 属性");
    return;
  }
  
  // 构建完整URL
  const fileUrl = new URL(href, location.href);
  const fileUrlStr = fileUrl.toString();
  
  // 从文件扩展名判断语言类型
  const pathnameParts = fileUrl.pathname.split(".");
  const fileExt = pathnameParts[pathnameParts.length - 1].toLowerCase();
  const lang = fileExt === "jsx" ? "jsx" : 
               fileExt === "tsx" ? "tsx" : 
               fileExt === "ts" ? "ts" : "js";
  
  // 读取导入映射
  let importMap: Record<string, any> = {};
  d.querySelectorAll('script[type="importmap"]').forEach((el) => {
    try {
      const content = el.textContent;
      if (content) {
        const parsed = JSON.parse(content);
        importMap = parsed;
      }
    } catch (e) {
      console.error("[esm.sh/xs] 导入映射解析错误", e);
    }
  });
  
  // 缓存控制选项
  const target = "$TARGET"; // 构建时注入
  const version = currentScript.getAttribute("version") || "";
  const noCache = currentScript.hasAttribute("no-cache") || new URL(location.href).searchParams.has("no-cache");
  const forceRefresh = currentScript.hasAttribute("refresh") || new URL(location.href).searchParams.has("refresh");
  const checkModified = currentScript.hasAttribute("check-modified");
  const maxAge = parseInt(currentScript.getAttribute("max-age") || "3600", 10) * 1000; // 转换为毫秒
  
  // 构建缓存相关的键
  const cachePrefix = "esm.sh/xs/";
  const fileKey = fileUrlStr + (version ? `?v=${version}` : "");
  const urlKey = cachePrefix + "url:" + fileKey;
  const contentKey = cachePrefix + "content:" + fileKey;
  const timeKey = cachePrefix + "time:" + fileKey;
  const etagKey = cachePrefix + "etag:" + fileKey;
  const lastModifiedKey = cachePrefix + "lastmod:" + fileKey;
  
  // 检查是否使用缓存
  let useCache = !noCache && !forceRefresh;
  let cachedCode: string | null = null;
  let cachedEtag: string | null = null;
  let cachedLastModified: string | null = null;
  let now = Date.now();

  try {
    if (useCache) {
      cachedCode = l.getItem(contentKey);
      cachedEtag = l.getItem(etagKey);
      cachedLastModified = l.getItem(lastModifiedKey);
      const cachedTime = l.getItem(timeKey);
      
      // 检查缓存是否过期
      if (cachedTime && (now - parseInt(cachedTime, 10)) > maxAge) {
        console.log("[esm.sh/xs] 缓存已过期");
        useCache = false;
        cachedCode = null;
      }
    }
  } catch (e) {
    // localStorage 可能被禁用
    useCache = false;
  }
  
  // 如果强制检查修改，且有缓存的 ETag 或 Last-Modified，则使用条件请求
  const headers: HeadersInit = {};
  if (checkModified && !forceRefresh && cachedCode) {
    if (cachedEtag) {
      headers["If-None-Match"] = cachedEtag;
    }
    if (cachedLastModified) {
      headers["If-Modified-Since"] = cachedLastModified;
    }
  }
  
  // 如果没有缓存，或者强制刷新，则从服务器获取
  if (!cachedCode || forceRefresh || checkModified) {
    try {
      // 从URL获取文件内容
      const fetchOptions: RequestInit = {
        headers,
        cache: forceRefresh ? "reload" : "default"
      };
      
      if (currentScript.hasAttribute("credentials")) {
        fetchOptions.credentials = "include";
      }
      
      // 添加版本号到URL查询参数，避免缓存
      const fetchUrl = new URL(fileUrlStr);
      if (version) {
        fetchUrl.searchParams.set("v", version);
      }
      
      const fileResponse = await fetch(fetchUrl.toString(), fetchOptions);
      
      // 如果使用条件请求且服务器返回304，表示文件未修改，继续使用缓存
      if (checkModified && fileResponse.status === 304 && cachedCode) {
        console.log("[esm.sh/xs] 文件未修改，使用缓存");
        // 更新缓存时间
        try {
          l.setItem(timeKey, now.toString());
        } catch (e) {
          // localStorage 可能被禁用
        }
      } else if (fileResponse.ok) {
        // 获取文件内容
        const sourceCode = await fileResponse.text();
        
        // 记录ETag和Last-Modified，用于下次的条件请求
        const etag = fileResponse.headers.get("ETag");
        const lastModified = fileResponse.headers.get("Last-Modified");
        
        // 转换代码
        let transformedCode: string;
        if (hostname === "localhost" || hostname === "127.0.0.1") {
          // 本地开发环境使用
          console.warn("[esm.sh/xs] 本地开发环境应使用 `npx esm.sh serve`");
          transformedCode = sourceCode; // 本地环境简单返回原始代码
        } else {
          // 生产环境处理
          
          // 计算源代码的哈希值，用于查找预编译的模块
          const buffer = new Uint8Array(
            await crypto.subtle.digest(
              "SHA-1",
              new TextEncoder().encode(lang + sourceCode + target + stringify(importMap) + "true"),
            ),
          );
          const hash = [...buffer].map((b) => b.toString(16).padStart(2, "0")).join("");
          
          // 尝试加载预编译的模块
          try {
            const prebuiltUrl = new URL(`/+${hash}.mjs`, currentScript.src);
            const prebuiltRes = await fetch(prebuiltUrl.toString(), {
              cache: forceRefresh ? "reload" : "default"
            });
            
            if (prebuiltRes.ok) {
              // 找到预编译的模块，直接使用
              console.log("[esm.sh/xs] 使用预编译模块");
              transformedCode = await prebuiltRes.text();
            } else {
              // 没有预编译的模块，调用transform API
              const transformUrl = new URL("/transform", currentScript.src);
              
              // 请求transform API
              const transformResponse = await fetch(transformUrl.toString(), {
                method: "POST",
                headers: {
                  "Content-Type": "application/json"
                },
                body: stringify({
                  filename: fileUrlStr,
                  lang,
                  code: sourceCode,
                  target,
                  importMap: Object.keys(importMap).length > 0 ? importMap : {},
                  minify: true
                })
              });
              
              if (!transformResponse.ok) {
                throw new Error(`转换失败: ${transformResponse.status} ${transformResponse.statusText}`);
              }
              
              const transformResult = await transformResponse.json();
              if (transformResult.error) {
                throw new Error(`转换错误: ${transformResult.error}`);
              }
              
              transformedCode = transformResult.code;
            }
          } catch (error) {
            // 预编译模块加载失败，使用transform API
            const transformUrl = new URL("/transform", currentScript.src);
            
            // 请求transform API
            const transformResponse = await fetch(transformUrl.toString(), {
              method: "POST",
              headers: {
                "Content-Type": "application/json"
              },
              body: stringify({
                filename: fileUrlStr,
                lang,
                code: sourceCode,
                target,
                importMap: Object.keys(importMap).length > 0 ? importMap : {},
                minify: true
              })
            });
            
            if (!transformResponse.ok) {
              throw new Error(`转换失败: ${transformResponse.status} ${transformResponse.statusText}`);
            }
            
            const transformResult = await transformResponse.json();
            if (transformResult.error) {
              throw new Error(`转换错误: ${transformResult.error}`);
            }
            
            transformedCode = transformResult.code;
          }
        }
        
        // 更新缓存
        cachedCode = transformedCode;
        try {
          l.setItem(contentKey, cachedCode);
          l.setItem(timeKey, now.toString());
          l.setItem(urlKey, fileUrlStr);
          
          if (etag) {
            l.setItem(etagKey, etag);
          }
          if (lastModified) {
            l.setItem(lastModifiedKey, lastModified);
          }
        } catch (e) {
          // localStorage 可能被禁用
        }
      } else {
        throw new Error(`无法获取文件: ${fileResponse.status} ${fileResponse.statusText}`);
      }
    } catch (error) {
      console.error("[esm.sh/xs] 处理文件失败:", error);
      const errorScript = d.createElement("script");
      errorScript.type = "module";
      errorScript.textContent = `console.error("[esm.sh/xs] 错误:", ${stringify(String(error))})`;
      currentScript.replaceWith(errorScript);
      return;
    }
  }
  
  // 执行已转换的代码
  try {
    // 为了更好的错误提示，添加 sourceURL 注释
    const execCode = cachedCode + `\n//# sourceURL=${fileUrlStr}`;
    const blob = new Blob([execCode], { type: "application/javascript" });
    const url = URL.createObjectURL(blob);
    const moduleScript = d.createElement("script");
    moduleScript.type = "module";
    moduleScript.src = url;
    d.head.appendChild(moduleScript);
    
    // 清理：移除一次性的脚本元素
    moduleScript.onload = moduleScript.onerror = () => {
      URL.revokeObjectURL(url);
      d.head.removeChild(moduleScript);
    };
  } catch (e) {
    console.error("[esm.sh/xs] 执行转换代码时出错:", e);
  }
}

run().catch(e => {
  console.error("[esm.sh/xs] 未捕获错误:", e);
}); 