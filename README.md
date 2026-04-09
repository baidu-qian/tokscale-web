# Tokscale Dashboard

一个基于 Go 的本地 Web 看板，用来把 `tokscale` CLI 的统计结果包装成可视化页面，方便查看不同 AI 客户端、模型、时间范围下的 Token 与费用消耗情况。

项目本身不直接计算成本数据，而是通过 HTTP API 调用 `npx tokscale`，再把结果展示成图表和明细表。

## 这个仓库是做什么的

这个仓库提供了一个轻量的本地分析面板，核心用途是：

- 将 `tokscale` 命令行结果封装为浏览器可访问的接口
- 展示总费用、总 Token、消息数、活跃天数、日均费用等核心指标
- 展示月度费用趋势、每日 Token 趋势、客户端占比、模型占比、月度 Token 构成、活动热力图
- 支持按时间范围筛选：`today`、`week`、`month`、`all`
- 支持按客户端筛选，例如 `claude`、`codex`、`gemini`、`cursor` 等

整体架构很简单：

- 后端：Go 单文件服务 [`main.go`](/data/infi/hongchun.you/tmp/tokscale-help/main.go)
- 前端：单页静态页面 [`static/index.html`](/data/infi/hongchun.you/tmp/tokscale-help/static/index.html)
- 运维脚本：[`service.sh`](/data/infi/hongchun.you/tmp/tokscale-help/service.sh)

## 依赖要求

运行这个项目至少需要以下依赖：

- Go 1.22+
- Node.js 与 `npx`
- 可执行的 `tokscale` CLI
- 浏览器

当前仓库的 Go 模块声明见 [`go.mod`](/data/infi/hongchun.you/tmp/tokscale-help/go.mod)：

```go
module tokscale-dashboard

go 1.22
```

额外说明：

- 前端图表依赖 ECharts，通过 CDN 在页面中加载
- 后端没有引入第三方 Go 包，主要使用标准库
- 如果本机无法执行 `npx tokscale`，页面接口会返回错误，图表也无法正常展示

## 项目如何工作

后端启动后监听 `http://0.0.0.0:8900`，并提供以下能力：

- `GET /`：返回内嵌的前端页面
- `GET /api/summary`：调用 `npx tokscale --json --no-spinner`
- `GET /api/monthly`：调用 `npx tokscale monthly --json --no-spinner`
- `GET /api/graph`：调用 `npx tokscale graph --no-spinner`

接口支持的查询参数：

- `range=all|today|week|month`
- `client=<client-name>`，仅 `summary` 接口支持

服务端对接口结果做了 60 秒内存缓存，避免频繁重复执行 CLI。

## 页面效果

页面是一个单页数据看板，默认深色主题，支持亮暗切换，主要包含：

- 顶部时间范围切换
- 客户端筛选下拉框
- 刷新按钮
- 总费用、总 Token、消息数、活跃天数、日均费用卡片
- 月度费用趋势图
- 每日 Token 使用趋势图
- 客户端费用占比图
- 模型费用占比图
- 月度 Token 构成图
- 每日活动热力图
- 详细明细表格，可排序查看客户端、模型、供应商、输入输出、缓存读写、消息数和费用

从仓库中的示例数据 [`full_graph.json`](/data/infi/hongchun.you/tmp/tokscale-help/full_graph.json) 可以看出，这个看板面向的是多客户端、多模型的 AI 使用成本分析场景。

## 快速开始

### 1. 确认依赖

先确认本机具备以下命令：

```bash
go version
node -v
npx tokscale
```

如果 `npx tokscale` 无法执行，需要先安装或配置对应的 `tokscale` 环境。

### 2. 本地编译运行

```bash
go build -o tokscale-dashboard .
./tokscale-dashboard
```

启动后访问：

```text
http://127.0.0.1:8900
```

注意：

- Linux 环境下程序会尝试调用 `xdg-open`
- macOS 会尝试调用 `open`
- Windows 会尝试自动打开浏览器

如果自动打开失败，手动访问即可。

### 3. 使用脚本管理

仓库附带了一个简单的服务脚本：

```bash
chmod +x service.sh
./service.sh start
./service.sh status
./service.sh test
./service.sh logs
./service.sh stop
```

脚本能力包括：

- `build`：编译二进制
- `start`：后台启动服务
- `stop`：停止服务
- `restart`：重启服务
- `status`：查看状态
- `test`：对首页和 API 做健康检查
- `logs`：查看最近日志

## 接口示例

获取全部汇总：

```bash
curl "http://127.0.0.1:8900/api/summary?range=all"
```

获取 Claude 客户端汇总：

```bash
curl "http://127.0.0.1:8900/api/summary?range=all&client=claude"
```

获取月度统计：

```bash
curl "http://127.0.0.1:8900/api/monthly?range=all"
```

获取图谱数据：

```bash
curl "http://127.0.0.1:8900/api/graph?range=month"
```

## 目录说明

```text
.
├── main.go                 # Go HTTP 服务，封装 tokscale CLI
├── static/index.html       # 单页前端看板
├── service.sh              # 启停、构建、测试脚本
├── full_graph.json         # 示例图谱/统计数据
├── tokscale-dashboard      # 已编译产物（Linux/Unix）
├── tokscale-dashboard.exe  # 已编译产物（Windows）
└── tokscale-dashboard.log  # 运行日志
```

## 已验证内容

当前我在本地验证了：

- `node -v` 可用
- 项目可通过 `go build ./...` 编译

编译时因为沙箱限制，需要把 `GOCACHE` 指到可写目录；真实本机环境一般不需要额外处理。

## 已知限制

- 数据来源完全依赖 `tokscale` CLI，本仓库不包含其实现
- 前端依赖公网 CDN 加载 ECharts，离线环境下图表可能无法显示
- 当前没有鉴权，默认面向本机使用
- 页面和接口端口固定为 `8900`
- 缓存为进程内缓存，服务重启后失效

## 适用场景

这个项目适合以下场景：

- 个人查看本机 AI 编程客户端的 Token 与费用使用情况
- 对比不同客户端或模型的成本占比
- 快速把 `tokscale` 的 CLI 数据变成更直观的浏览器看板

