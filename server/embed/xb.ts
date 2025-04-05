/*!
 * ⚡️ esm.sh/x - ts/jsx/vue/svelte just works™️ in browser.
 * Usage: <script type="module" src="app.tsx"> → <script src="https://esm.sh/x" href="app.tsx">
 * 
 * 支持的属性:
 * - href: 要加载的脚本URL (必需)
 * - deno-json: 指定deno.json的路径，用于获取importmap (可选)
 * - refresh: 当设置为"true"时，强制刷新缓存 (可选)
 */

((document, location) => {
  const currentScript = document.currentScript as HTMLScriptElement | null;
  const $: typeof document.querySelector = (s: string) => document.querySelector(s);
  if (location.protocol == "file:" || ["localhost", "127.0.0.1"].includes(location.hostname)) {
    console.error("[esm.sh/x] Please start your app with `npx esm.sh serve` in development env.");
    return;
  }
  let main = currentScript?.getAttribute("href");
  if (main) {
    const mainUrl = new URL(main, location.href);
    const ctx = btoa(location.pathname).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
    const v = $<HTMLMetaElement>("meta[name=version]")?.content;
    const { searchParams, pathname } = mainUrl;
    
    // 检查是否指定了deno-json
    const denoJsonPath = currentScript?.getAttribute("deno-json");
    if (denoJsonPath) {
      // 对路径进行 base64 编码
      const encodedPath = btoa(denoJsonPath).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
      searchParams.set("dj", encodedPath);
    }
    
    // 检查是否需要强制刷新缓存
    const refresh = currentScript?.hasAttribute("refresh");
    if (refresh) {
      // 添加时间戳或设置refresh=true来强制刷新缓存
      searchParams.set("refresh", Date.now().toString());
    }
    
    if (pathname.endsWith("/uno.css")) {
      searchParams.set("ctx", ctx);
    } else if ($("script[type=importmap]")) {
      searchParams.set("im", ctx);
    }
    if (v) {
      searchParams.set("v", v);
    }
    main = new URL(currentScript!.src).origin + "/" + mainUrl;
    if (pathname.endsWith(".css")) {
      currentScript!.insertAdjacentHTML("afterend", `<link rel="stylesheet" href="${main}">`);
    } else {
      import(main);
    }
  }
})(document, location);
