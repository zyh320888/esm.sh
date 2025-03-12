# 自托管

[esm.sh](https://esm.sh) 提供了一个由 [Cloudflare](https://cloudflare.com) 支持的全球快速 CDN。
你也可以自行托管 esm.sh 服务。为此，请按照以下说明操作。

## 克隆源代码

```bash
git clone https://github.com/esm-dev/esm.sh
cd esm.sh
```

## 配置

要配置服务器，请创建一个 `config.json` 文件，然后将其传递给服务器启动命令。例如：

```jsonc
// config.json
{
  "port": 8080,
  "npmRegistry": "https://registry.npmjs.org/",
  "npmToken": "******"
}
```

你可以在 [config.example.jsonc](./config.example.jsonc) 中找到所有服务器选项。

## 本地运行服务器

你需要 [Go](https://golang.org/dl) 1.22+ 来编译和运行服务器。

```bash
go run server/cmd/main.go --config=config.json
```

然后你可以从 <http://localhost:8080/react> 导入 `React`。

## 部署到单台机器

你可以使用 [deploy.sh](./scripts/deploy.sh) 脚本将服务器部署到单台机器上。

```bash
# 首次部署
./scripts/deploy.sh --init
# 更新服务器
./scripts/deploy.sh
```

推荐的托管要求：

- 安装有 systemd 的 Linux 系统
- 4 核 CPU 或更多
- 8GB 内存或更多
- 100GB 磁盘空间或更多

## 使用 Docker 部署

[![Docker 镜像](https://img.shields.io/github/v/tag/esm-dev/esm.sh?label=Docker&display_name=tag&sort=semver&style=flat&colorA=232323&colorB=232323&logo=docker&logoColor=eeeeee)](https://github.com/esm-dev/esm.sh/pkgs/container/esm.sh)

esm.sh 提供了一个 Docker 镜像用于快速部署。你可以从 <https://ghcr.io/esm-dev/esm.sh> 拉取容器镜像。

```bash
docker pull ghcr.io/esm-dev/esm.sh      # 最新版本
docker pull ghcr.io/esm-dev/esm.sh:v136 # 特定版本
```

运行容器：

```bash
docker run -p 8080:8080 \
  -e NPM_REGISTRY=https://registry.npmjs.org/ \
  -e NPM_TOKEN=****** \
  -v MY_VOLUME:/esmd \
  ghcr.io/esm-dev/esm.sh:latest
```

可用的环境变量：

- `COMPRESS`: 使用 gzip/brotli 压缩 HTTP 响应，默认为 `true`。
- `CUSTOM_LANDING_PAGE_ORIGIN`: 自定义着陆页来源，默认为空。
- `CUSTOM_LANDING_PAGE_ASSETS`: 自定义着陆页资源，以逗号分隔，默认为空。
- `CORS_ALLOW_ORIGINS`: CORS 允许的来源，以逗号分隔，默认为允许所有来源。
- `LOG_LEVEL`: 日志级别，可用值为 ["debug", "info", "warn", "error"]，默认为 "info"。
- `ACCESS_LOG`: 启用访问日志，默认为 `false`。
- `MINIFY`: 压缩构建的 JS/CSS 文件，默认为 `true`。
- `NPM_QUERY_CACHE_TTL`: NPM 查询的缓存 TTL，默认为 10 分钟。
- `NPM_REGISTRY`: 全局 NPM 注册表，默认为 "https://registry.npmjs.org/"。
- `NPM_TOKEN`: 全局 NPM 注册表的访问令牌。
- `NPM_USER`: 全局 NPM 注册表的访问用户。
- `NPM_PASSWORD`: 全局 NPM 注册表的访问密码。
- `SOURCEMAP`: 为构建的 JS/CSS 文件生成源映射，默认为 `true`。
- `STORAGE_TYPE`: 存储类型，可用值为 ["fs", "s3"]，默认为 "fs"。
- `STORAGE_ENDPOINT`: 存储端点，默认为 "~/.esmd/storage"。
- `STORAGE_REGION`: S3 存储的区域。
- `STORAGE_ACCESS_KEY_ID`: S3 存储的访问密钥。
- `STORAGE_SECRET_ACCESS_KEY`: S3 存储的密钥。

你也可以基于 `ghcr.io/esm-dev/esm.sh` 创建自己的 Dockerfile：

```dockerfile
FROM ghcr.io/esm-dev/esm.sh:latest
ADD --chown=esm:esm ./config.json /etc/esmd/config.json
CMD ["esmd", "--config", "/etc/esmd/config.json"]
```

## 使用 CloudFlare CDN 部署

要使用 CloudFlare CDN 部署服务器，你需要在 CloudFlare 仪表板中创建以下缓存规则（参见 [链接](https://developers.cloudflare.com/cache/how-to/cache-rules/create-dashboard/)），并且每个规则应设置为 **"符合缓存条件"**：

#### 1. 缓存 `.d.ts` 文件

```ruby
(ends_with(http.request.uri.path, ".d.ts")) or
(ends_with(http.request.uri.path, ".d.mts")) or
(ends_with(http.request.uri.path, ".d.cts"))
```

#### 2. 缓存包资源

```ruby
(http.request.uri.path.extension in {"node" "wasm" "less" "sass" "scss" "stylus" "styl" "json" "jsonc" "csv" "xml" "plist" "tmLanguage" "tmTheme" "yml" "yaml" "txt" "glsl" "frag" "vert" "md" "mdx" "markdown" "html" "htm" "svg" "png" "jpg" "jpeg" "webp" "gif" "ico" "eot" "ttf" "otf" "woff" "woff2" "m4a" "mp3" "m3a" "ogg" "oga" "wav" "weba" "gz" "tgz" "css" "map"})
```

#### 3. 缓存 `?target=*`

```ruby
(http.request.uri.query contains "target=es2015") or
(http.request.uri.query contains "target=es2016") or
(http.request.uri.query contains "target=es2017") or
(http.request.uri.query contains "target=es2018") or
(http.request.uri.query contains "target=es2019") or
(http.request.uri.query contains "target=es2020") or
(http.request.uri.query contains "target=es2021") or
(http.request.uri.query contains "target=es2022") or
(http.request.uri.query contains "target=es2023")or
(http.request.uri.query contains "target=es2024") or
(http.request.uri.query contains "target=esnext") or
(http.request.uri.query contains "target=denonext") or
(http.request.uri.query contains "target=deno") or
(http.request.uri.query contains "target=node")
```

#### 4. 缓存 `/(target)/`

```ruby
(http.request.uri.path contains "/es2015/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/es2016/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/es2017/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/es2018/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/es2019/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/es2020/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/es2021/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/es2022/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/es2023/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/es2024/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/esnext/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/denonext/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/deno/" and http.request.uri.path.extension in {"mjs" "map" "css"}) or
(http.request.uri.path contains "/node/" and http.request.uri.path.extension in {"mjs" "map" "css"})
```

#### 5. 绕过 Deno/Bun/Node 的缓存

```ruby
(not starts_with(http.user_agent, "Deno/") and not starts_with(http.user_agent, "Bun/") and not starts_with(http.user_agent, "Node/") and not starts_with(http.user_agent, "Node.js/") and http.user_agent ne "undici")
```

> [!NOTE]
> 由于 Cloudflare 不尊重 `Vary` 头，我们需要为 `Deno`/`Bun`/`Node` 客户端绕过缓存。
