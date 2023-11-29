/** @version: 0.57.7 */

const eventColors = {
  modify: "#056CF0",
  create: "#20B44B",
  remove: "#F00C08",
};

export default {
  name: "hmr",
  devOnly: true,
  setup(hot: any) {
    hot.hmr = true;
    hot.hmrModules = new Set<string>();
    hot.hmrCallbacks = new Map<string, (module: any) => void>();
    hot.customImports = {
      ...hot.customImports,
      "@hmrRuntimeUrl": "https://esm.sh/hot/_hmr.js",
      "@reactRefreshRuntimeUrl": "https://esm.sh/hot/_hmr_react.js",
    };
    hot.register("_hmr.js", () => "", () => {
      return `
        export default (path) => ({
          decline() {
            HOT.hmrModules.delete(path);
            HOT.hmrCallbacks.set(path, () => location.reload());
          },
          accept(cb) {
            const hmrModules = HOT.hmrModules ?? (HOT.hmrModules = new Set());
            const hmrCallbacks = HOT.hmrCallbacks ?? (HOT.hmrCallbacks = new Map());
            if (!HOT.hmrModules.has(path)) {
              HOT.hmrModules.add(path);
              HOT.hmrCallbacks.set(path, cb);
            }
          },
          invalidate() {
            location.reload();
          }
        })
      `;
    });
    hot.register("_hmr_react.js", () => "", () => {
      return `
        // react-refresh
        // @link https://github.com/facebook/react/issues/16604#issuecomment-528663101

        import runtime from "https://esm.sh/v135/react-refresh@0.14.0/runtime";

        let timer;
        const refresh = () => {
          if (timer !== null) {
            clearTimeout(timer);
          }
          timer = setTimeout(() => {
            runtime.performReactRefresh()
            timer = null;
          }, 30);
        };

        runtime.injectIntoGlobalHook(window);
        window.$RefreshReg$ = () => {};
        window.$RefreshSig$ = () => type => type;

        export { refresh as __REACT_REFRESH__, runtime as __REACT_REFRESH_RUNTIME__ };
      `;
    });
    hot.onFire((_sw: ServiceWorker) => {
      const source = new EventSource(new URL("hot-notify", location.href));
      source.addEventListener("fs-notify", async (ev) => {
        const { type, name } = JSON.parse(ev.data);
        const module = hot.hmrModules.has(name);
        const handler = hot.hmrCallbacks.get(name);
        if (type === "modify") {
          if (module) {
            const url = new URL(name, location.href);
            url.searchParams.set("t", Date.now().toString(36));
            if (url.pathname.endsWith(".css")) {
              url.searchParams.set("module", "");
            }
            const module = await import(url.href);
            if (handler) {
              handler(module);
            }
          } else if (handler) {
            handler();
          }
        }
        if (module || handler) {
          console.log(
            `🔥 %c[HMR] %c${type}`,
            "color:#999",
            `color:${eventColors[type as keyof typeof eventColors]}`,
            `${JSON.stringify(name)}`,
          );
        }
      });
      source.onopen = () => {
        console.log(
          "🔥 %c[HMR]",
          "color:#999",
          "listening for file changes...",
        );
      };
      source.onerror = (err) => {
        if (err.eventPhase === EventSource.CLOSED) {
          console.log(
            "🔥 %c[HMR]",
            "color:#999",
            "connection lost, reconnecting...",
          );
        }
      };
    });
  },
};
