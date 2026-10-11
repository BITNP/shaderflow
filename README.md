# ShaderFlow

ShaderFlow 是一个多人 **WebGPU Shader Wall**：每位已认证的玩家提交一个受锁定模板约束的 WGSL 函数；管理大屏验证后，将玩家的动画绘制到现场画面的一个正方形格子中。

## 工作方式

1. 玩家通过部署在 ShaderFlow 前面的 **oauth2-proxy** 完成登录。
2. 玩家只能编辑两个颜色常量和 `userfn(x, y, t)`；浏览器在本地用完整锁定 WGSL 模板编译并绘制。
3. 玩家提交的是候选版本。候选版本不会直接成为现场代码，即使有人绕过浏览器校验直接请求 API 也是如此。
4. 管理大屏再次用 WebGPU 编译并实际绘制候选版本；通过后以管理员 Cookie 调用确认接口，服务端才将其提升为已接受版本。
5. 已接受且在线的玩家占用一个正方形。布局保留空格优先填入；满格时重新排布；玩家离开后空位超过容量四分之一时重新排布。

玩家页面与管理页面分别位于 `/player/` 和 `/admin/`。访问根路径 `/` 时：已登录玩家自动跳转到 `/player/`，已登录管理员自动跳转到 `/admin/`，未认证请求跳转到 `/player/`。普通用户不会看到管理入口；管理员自行访问 `/admin/` 并输入管理员密码。

## 配置与启动

在 JSON 配置中设置管理员密码，例如：

```json
{
  "adminPassword": "replace-with-a-long-secret"
}
```

使用 `server.dev.example.json` 启动：

```sh
go run ./cmd/shaderflow -config server.dev.example.json
```

`adminPassword` 直接从 JSON 配置读取。管理员通过 `/admin/` 输入该密码，服务端使用 HttpOnly、SameSite=Strict Cookie 保存管理员会话。请限制生产配置文件的读取权限，并且不要将真实密码提交到版本库。

## oauth2-proxy

应用不处理 OIDC 协议。生产部署使用仓库根目录的 `docker-compose.yml` 启动 oauth2-proxy；它是应用的唯一入口，并将认证用户透传到上游。OAuth2 Proxy 需要启用：

```text
--pass-user-headers=true
```

ShaderFlow 从 oauth2-proxy 请求头 `X-Forwarded-User` 获取玩家身份。常见的反向代理配置还会使用 oauth2-proxy 的 `--set-xauthrequest`，但本应用读取的是传给上游的 `X-Forwarded-User`，不是浏览器可伪造的请求头。

### Docker + OIDC 生产部署

根目录的 `docker-compose.yml` 运行应用和 [oauth2-proxy](https://oauth2-proxy.github.io/oauth2-proxy/)。应用没有宿主机端口；oauth2-proxy 仅绑定到 `127.0.0.1:4180`，应由另一个 HTTPS 反向代理转发。将反向代理的公网 HTTPS 地址加上 `/oauth2/callback` 配置为 OIDC 客户端的回调地址。

在启动前于 `conf/` 创建受限权限的 `server.docker.json` 和 `oauth2-proxy.cfg`。前者应设置生产管理员密码、`cookieSecure: true` 和 `trustedProxyCIDRs: ["172.30.0.2/32"]`；后者是 oauth2-proxy 的原生配置文件，应配置 OIDC provider、issuer、client、cookie secret、redirect URL、`http_address = "0.0.0.0:4180"`、上游 `http://app:8080/`、`pass_user_headers = true`、`reverse_proxy = true` 与 `cookie_secure = true`。两份配置文件均被 Git 忽略。

```sh
docker compose up --build -d
```

不要放宽应用的 `trustedProxyCIDRs`，也不要为 `app` 添加 `ports`。生产配置中的 `cookieSecure` 必须保持 `true`，并且外部反向代理必须使用 HTTPS。

### 多用户本机开发（Caddy）

仓库包含 `docker-compose.dev.yml`、`conf/Caddyfile`、`conf/server.docker.dev.json` 以及运行数据目录 `data/`，用于模拟九个 oauth2-proxy 已认证用户。`conf/` 以只读 bind mount 提供给应用和 Caddy，`data/` 以可写 bind mount 持久化管理员 Session。Caddy 将 `user1.localhost` 到 `user9.localhost` 的请求分别代理到同一个应用容器，并添加对应的 `X-Forwarded-User` 请求头；`admin.localhost` 不注入玩家身份，只用于管理员登录。此配置仅用于本机开发，不提供 OIDC 或生产级认证。

```sh
docker compose -f docker-compose.dev.yml up --build
```

然后使用不同的普通/无痕浏览器窗口访问：

- `http://admin.localhost/admin/console/`：管理控制台
- `http://admin.localhost/admin/view/`：大屏（无需全屏也可显示；右上角可切换浏览器全屏）
- `http://user1.localhost/player/`
- `http://user2.localhost/player/`
- `http://user9.localhost/player/`

管理控制台和大屏已拆分：`/admin/` 会重定向到 `/admin/console/`；大屏位于 `/admin/view/`。大屏在普通窗口和浏览器全屏模式下都会渲染；右上角低透明度控件可切换全屏或返回控制台。

开发管理员密码为 `dev-admin-change-me`，定义在 `conf/server.docker.dev.json`。Caddy 暴露宿主机的 8080 端口；若该端口已被占用，请修改 `docker-compose.dev.yml` 的端口映射，并在 URL 中加上该端口。此开发配置只用于模拟可信身份，不能用于生产。


## API 概览

- `GET /api/auth/me`：管理员 Cookie 或可信代理用户身份。
- `GET /api/state`：管理员得到全部玩家和候选版本；玩家只得到自己的版本和格子。
- `POST /api/candidate`：玩家上传 `{version, source}` 候选版本。
- `POST /api/admin/candidates/{sub}/promote`：管理大屏验证成功后确认指定版本。
- `POST /api/admin/players/{sub}/kick`：立即停止展示该玩家；只有其提交更高版本并通过自动验证后才会重新加入。
- `PUT /api/admin/metrics`：管理大屏每秒批量上报每格的 GPU 时间（若 `timestamp-query` 可用）及 CPU 编码/提交时间。
- `PUT /api/admin/viewport`：由 `/admin/view/` 自动提交当前浏览器视口的 `{width,height}`，并重新计算格子布局。
- `GET /api/ws`：状态同步及在线检测。玩家断开 WebSocket 后有 30 秒宽限期。

## 架构

```text
oauth2-proxy -- X-Forwarded-User --> Go 服务
                                      ├─ internal/server: 管理员会话、可信代理认证、API/WS
                                      └─ internal/game: 候选→确认状态机、在线状态、正方形排布
浏览器
├─ web/player: 固定 WGSL 模板的本地验证和候选提交
├─ web/admin: 权威验证、确认候选、各格子 WebGPU 绘制
└─ web/shared/wall.js: 固定 wrapper、可编辑区解析、WebGPU render pipeline
```
