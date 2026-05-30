<p align="center">
  <img src="./docs/img/logo.png" alt="prxy logo" width="75%" height="75%>
</p>
<p align="center">
  </br>
  <b>prxy</b> 是一个用 <a href="https://go.dev/">Go</a> 编写的多功能 HTTP 反向代理，支持批量代理配置、可选的出站代理路由、自动 Host 头重写和智能 HTTP 响应缓存。
</p>
<hr>

[![最新发布](https://img.shields.io/github/v/tag/Madh93/prxy?label=Release)](https://github.com/Madh93/prxy/releases)
[![Go 版本](https://img.shields.io/badge/Go-1.24-blue)](https://go.dev/doc/install)
[![Go 报告卡](https://goreportcard.com/badge/github.com/Madh93/prxy)](https://goreportcard.com/report/github.com/Madh93/prxy)
[![构建状态](https://img.shields.io/github/actions/workflow/status/Madh93/prxy/continuous-integration.yml?branch=main)](https://github.com/Madh93/prxy/actions)
[![Go 文档](https://pkg.go.dev/badge/github.com/Madh93/prxy.svg)](https://pkg.go.dev/github.com/Madh93/prxy)
[![许可证](https://img.shields.io/badge/License-MIT-brightgreen)](LICENSE)

<p align="center">
  <a href="#功能特性">功能特性</a> •
  <a href="#安装">安装</a> •
  <a href="#快速开始">快速开始</a> •
  <a href="#配置">配置</a> •
  <a href="#缓存">缓存</a> •
  <a href="#开发">开发</a> •
  <a href="#许可证">许可证</a>
  <br>
  <a href="README.md">English</a>
</p>

## 功能特性

### 核心功能

- **批量代理配置**：同时运行多个代理服务，每个服务监听不同端口，转发到不同目标 URL
- **出站代理支持**：通过外部 HTTP 代理（如 wireproxy、Squid）路由所有代理流量
- **自动 Host 头重写**：确保请求到达正确的目标服务
- **优雅关闭**：处理 SIGTERM/SIGINT 信号以实现优雅关闭

### 高级缓存

- **基于路径的存储**：使用 URL 路径存储缓存文件，便于检查
- **流式缓存支持**：通过流式写入高效处理大文件
- **Range 请求支持**：完全支持 HTTP Range 请求以实现部分内容传输
- **可配置的缓存策略**：
  - 文件大小限制（最小/最大）
  - 总缓存大小限制
  - 基于扩展名的排除
  - 认证请求缓存（可选）
- **GitHub Releases 优化**：针对 GitHub 版本下载的特殊处理，提升性能
- **缓存统计**：跟踪缓存命中、未命中和存储使用情况

### 配置管理

- **多种配置源**：CLI 标志、环境变量和配置文件
- **配置持久化**：自动将设置保存到配置文件
- **灵活的路由管理**：通过 CLI 或配置文件添加路由
- **动态路由添加**：无需编辑配置文件即可临时添加路由

## 安装

### 二进制文件

从 [发布页面](https://github.com/Madh93/prxy/releases) 下载最新二进制文件：

```bash
curl -L https://github.com/Madh93/prxy/releases/latest/download/prxy_$(uname -s)_$(uname -m).tar.gz | tar -xz -O prxy > /usr/local/bin/prxy
chmod +x /usr/local/bin/prxy
```

### 从源码构建

```bash
go install github.com/Madh93/prxy@latest
```

### Docker

```bash
docker run --name prxy ghcr.io/madh93/prxy:latest --proxy http://proxy:8080
```

### Docker Compose

```yaml
services:
  prxy:
    image: ghcr.io/madh93/prxy:latest
    restart: unless-stopped
    volumes:
      - ./config.json:/root/cache/prxy.json:ro
      - ./cache:/root/cache
    environment:
      - PRXY_PROXY=http://proxy:8080
```

## 快速开始

### 单目标模式

```bash
prxy --target https://example.com --proxy http://localhost:8080 --port 8080
```

### 批量模式（推荐）

在 `./cache/prxy.json` 创建配置文件：

```json
{
  "host": "0.0.0.0",
  "proxy": "http://localhost:8080",
  "routes": [
    {"target": "https://github.com", "port": 8081},
    {"target": "https://gitlab.com", "port": 8082},
    {"target": "https://api.example.com", "port": 8083}
  ],
  "cache": {
    "enabled": true,
    "directory": "./cache"
  }
}
```

启动服务：

```bash
prxy
```

### 添加临时路由

通过 CLI 添加路由而无需编辑配置文件：

```bash
prxy --target https://httpbin.org --port 9999
```

## 配置

### 配置文件

配置文件在首次运行时自动创建于 `./cache/prxy.json`。默认结构：

```json
{
  "host": "0.0.0.0",
  "proxy": "",
  "routes": [],
  "cache": {
    "enabled": true,
    "directory": "./cache",
    "maxTotalSizeMB": 0,
    "minFileSizeKB": 0,
    "maxFileSizeKB": 0,
    "cacheAuth": false,
    "excludeExtensions": ["html", "js", "css", "json", "xml"]
  },
  "logging": {
    "level": "info",
    "format": "text",
    "output": "stdout"
  }
}
```

### 配置优先级

配置值从多个来源加载（优先级从高到低）：

1. **命令行标志**
2. **环境变量**（`PRXY_*` 前缀）
3. **配置文件**（`./cache/prxy.json`）
4. **默认值**

### CLI 标志

| 标志 | 环境变量 | 描述 | 默认值 |
|------|----------|------|--------|
| `--config`, `-c` | `PRXY_CONFIG` | 配置文件路径 | `./cache/prxy.json` |
| `--target`, `-t` | `PRXY_TARGET` | 目标服务 URL | N/A |
| `--port`, `-P` | `PRXY_PORT` | 监听端口 | N/A |
| `--proxy`, `-x` | `PRXY_PROXY` | 出站 HTTP 代理 URL | 直连 |
| `--host`, `-H` | `PRXY_HOST` | 监听主机 | `0.0.0.0` |
| `--cache` | `PRXY_CACHE` | 启用缓存 | `true` |
| `--clear-cache` | N/A | 清除缓存并退出 | `false` |
| `--yes` | `PRXY_YES` | 自动确认清除缓存 | `false` |
| `--log-level`, `-l` | `PRXY_LOG_LEVEL` | 日志级别（debug/info/warn/error） | `info` |
| `--log-format`, `-f` | `PRXY_LOG_FORMAT` | 日志格式（text/json） | `text` |
| `--log-output`, `-o` | `PRXY_LOG_OUTPUT` | 日志输出（stdout/stderr/file） | `stdout` |

**注意**：`--target` 和 `--port` 会添加额外的路由而不是替换现有路由。重复的端口会自动跳过并显示警告。

## 缓存

### 缓存配置

缓存行为通过配置文件中的 `cache` 部分控制：

- **enabled**：启用或禁用缓存
- **directory**：缓存文件存储目录
- **maxTotalSizeMB**：最大总缓存大小（0 = 无限制，> 0 时启用 LRU 驱逐）
- **minFileSizeKB**：要缓存的最小文件大小（0 = 无限制）
- **maxFileSizeKB**：要缓存的最大文件大小（0 = 无限制）
- **cacheAuth**：是否缓存认证请求
- **excludeExtensions**：要从缓存中排除的文件扩展名

### 缓存管理

查看缓存统计：

```bash
# 缓存统计信息会自动记录
# 检查响应头以获取缓存状态：
# X-Cache: HIT     - 来自缓存的响应
# X-Cache: MISS    - 转发到目标的响应
# X-Cache: BYPASS  - 请求未缓存（被策略排除）
```

清除缓存：

```bash
# 交互式确认
prxy --clear-cache

# 跳过确认
prxy --clear-cache --yes
```

### 缓存存储

缓存文件使用 URL 路径存储，便于检查：

```
cache/
├── prxy.json          # 配置文件
└── data/
    ├── github.com/
    │   └── releases/
    │       └── file.tar.gz
    └── api.example.com/
        └── endpoint.json
```

### GitHub Releases 优化

GitHub releases 下载自动优化：
- 高效的流式缓存写入
- 自动重定向跟随
- 改进的连接处理

## 开发

### 构建

```bash
make build              # 为当前平台构建
make build-all          # 为所有平台构建
make dev                # 快速开发构建
```

### 测试

```bash
make test               # 运行所有测试
make test-cover         # 运行测试并生成覆盖率报告
./run_all_tests.sh      # 运行集成测试
```

### 代码质量

```bash
make lint               # 运行 golangci-lint
make fmt                # 格式化代码
make vet                # 运行 go vet
```

### 开发工作流程

1. Fork 仓库
2. 创建功能分支
3. 进行更改
4. 运行测试和代码检查
5. 提交 Pull Request

## 架构

### 包结构

```
internal/
├── cache/      # HTTP 缓存层，基于路径存储
├── config/     # 配置管理和文件持久化
├── logging/    # 基于 slog 的结构化日志包装器
├── prxy/       # 核心反向代理逻辑和批量管理
└── validation/ # URL 和配置验证
```

### 关键组件

- **PrxyManager**：管理多个代理服务器实例
- **Prxy**：处理 HTTP 请求的独立代理服务器
- **Cache**：支持流式写入的 HTTP 响应缓存
- **Config**：配置加载和验证

### 依赖

- Go 1.24.3 或更高版本
- 仅一个外部依赖：`github.com/spf13/pflag`

## 使用场景

### 1. 访问家庭实验室服务

通过 WireGuard VPN 的 wireproxy 访问自托管服务，而不路由所有流量：

```bash
# 启动 wireproxy
wireproxy -config wireguard.conf

# prxy 通过 wireproxy 路由流量
prxy --proxy http://127.0.0.1:25345
```

### 2. 为不支持代理的应用启用代理

浏览器扩展或简单客户端可能没有代理设置。使用 prxy 创建本地代理：

```json
{
  "proxy": "http://127.0.0.1:25345",
  "routes": [
    {"target": "https://karakeep.my-homelab.tld", "port": 8080}
  ]
}
```

配置应用连接到 `http://localhost:8080`，prxy 会透明地转发请求。

### 3. 开发环境代理

多个开发服务通过单个出站代理：

```json
{
  "proxy": "http://company-proxy:8080",
  "routes": [
    {"target": "https://api.dev.internal", "port": 8081},
    {"target": "https://web.dev.internal", "port": 8082},
    {"target": "https://admin.dev.internal", "port": 8083}
  ]
}
```

### 4. 软件包镜像加速

为包管理器创建本地镜像缓存：

```json
{
  "routes": [
    {"target": "https://mirrors.aliyun.com", "port": 8080}
  ],
  "cache": {
    "enabled": true,
    "directory": "./cache",
    "maxTotalSizeMB": 10000
  }
}
```

配置包管理器使用 `http://localhost:8080` 作为镜像源。

## 许可证

本项目采用 [MIT 许可证](LICENSE)。