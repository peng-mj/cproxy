<p align="center">
  <img src="./docs/img/logo.png" alt="scproxy logo" width="60%" height="60%">
</p>
<p align="center">
  </br>
  <b>scproxy</b> — 带智能缓存的命令行 HTTP 反向代理
</p>
<p align="center">
  从 <a href="https://github.com/Madh93/prxy">Madh93/prxy</a> Fork 并增强功能
</p>

<p align="center">
  <i>scproxy 全称为 <b>Sieve Cache Proxy</b>（筛子缓存代理）—— 高效筛选和缓存 HTTP 响应。</i>
</p>

[![Latest release](https://img.shields.io/github/v/tag/peng-mj/scproxy?label=Release)](https://github.com/peng-mj/scproxy/releases)
[![Go Version](https://img.shields.io/badge/Go-1.24-blue)](https://go.dev/doc/install)
[![Go Report Card](https://goreportcard.com/badge/github.com/peng-mj/scproxy)](https://goreportcard.com/report/github.com/peng-mj/scproxy)
[![Build Status](https://img.shields.io/github/actions/workflow/status/peng-mj/scproxy/continuous-integration.yml?branch=main)](https://github.com/peng-mj/scproxy/actions)
[![License](https://img.shields.io/badge/License-MIT-brightgreen)](LICENSE)

<p align="center">
  <a href="#features">功能特性</a> •
  <a href="#installation">安装</a> •
  <a href="#quick-start">快速开始</a> •
  <a href="#configuration">配置</a> •
  <a href="#caching">缓存</a> •
  <a href="#license">许可证</a>
  <br>
  <a href="README.md">English</a>
</p>

---

## 概述

**scproxy** 是一个用 Go 编写的轻量级 HTTP 反向代理，帮助您通过出站代理转发 HTTP 请求，并提供智能响应缓存。适用于以下场景：

- 通过不同本地端口路由多个服务
- 通过外部 HTTP 代理（如企业代理、wireproxy、Squid）转发流量
- 缓存 HTTP 响应以减少带宽并降低延迟

---

## 功能特性

### 核心功能

- **批量代理模式** — 同时在多个端口运行多条路由
- **出站代理链** — 通过外部 HTTP 代理转发所有流量
- **自动 Host 头重写** — 确保请求到达正确的后端
- **优雅关闭** — 妥善处理 SIGTERM/SIGINT 信号

### 缓存功能

- **基于路径的存储** — 缓存文件镜像 URL 路径，便于检查
- **流式支持** — 高效处理大文件
- **HTTP Range 请求** — 完整支持部分内容传递
- **可配置策略** — 大小限制、扩展名排除、认证处理
- **LRU 淘汰** — 达到大小限制时自动清理缓存
- **缓存统计** — 跟踪命中、未命中和存储使用情况

### 配置管理

- **多种来源** — CLI 标志、配置文件、默认值
- **动态路由** — 通过 CLI 添加路由，无需编辑配置
- **自动保存** — CLI 设置持久化到配置文件

---

## 安装

### 二进制（推荐）

```bash
curl -L https://github.com/peng-mj/scproxy/releases/latest/download/scproxy_$(uname -s)_$(uname -m).tar.gz | tar -xz -O scproxy > /usr/local/bin/scproxy
chmod +x /usr/local/bin/scproxy
```

### 从源码安装

```bash
go install github.com/peng-mj/scproxy@latest
```

---

## 快速开始

### 单路由模式

通过出站代理转发单个目标：

```bash
scproxy --target https://example.com --proxy http://proxy:8080 --port 8080
```

### 批量模式（多路由）

在 `./cache/scproxy.json` 创建配置：

```json
{
  "host": "0.0.0.0",
  "proxy": "http://proxy:8080",
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

然后运行：

```bash
scproxy
```

### 添加临时路由

```bash
scproxy --target https://httpbin.org --port 9999
```

---

## 配置

### 配置文件

首次运行时自动创建于 `./cache/scproxy.json`：

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
    "excludeExtensions": ["html", "js", "css", "json", "xml"],
    "excludePaths": []
  },
  "logging": {
    "level": "info",
    "format": "text",
    "output": "stdout"
  }
}
```

### 优先级

1. CLI 标志（最高）
2. 配置文件
3. 默认值（最低）

### CLI 选项

| 标志 | 描述 | 默认值 |
|------|------|--------|
| `--target`, `-t` | 目标服务 URL | — |
| `--port`, `-P` | 监听端口 | — |
| `--proxy`, `-x` | 出站代理 URL | 直连 |
| `--host`, `-H` | 监听主机 | 0.0.0.0 |
| `--config`, `-c` | 配置文件路径 | ./cache/scproxy.json |
| `--cache` | 启用缓存 | true |
| `--clear-cache` | 清空缓存并退出 | — |
| `--yes` | 跳过确认 | — |
| `--summary`, `-s` | 显示缓存统计 | — |
| `--version`, `-v` | 显示版本 | — |
| `--log-level`, `-l` | 日志级别 | info |
| `--log-output`, `-o` | 日志输出 | stdout |

---

## 缓存

### 缓存控制

| 设置 | 描述 |
|------|------|
| `enabled` | 启用/禁用缓存 |
| `directory` | 存储路径 |
| `maxTotalSizeMB` | 总大小限制（0 = 无限制，设置后启用 LRU） |
| `minFileSizeKB` | 最小缓存文件大小 |
| `maxFileSizeKB` | 最大缓存文件大小 |
| `cacheAuth` | 缓存认证请求 |
| `excludeExtensions` | 跳过的文件扩展名 |
| `excludePaths` | 跳过的 URL 路径模式 |

### 路径排除

`excludePaths` 支持两种模式：

| 模式 | 匹配内容 |
|------|----------|
| `/ubuntu/dists/` | 此目录下的所有路径（前缀匹配） |
| `/etc/resolv.conf` | 仅此文件（精确匹配） |

规则：
- 必须以 `/` 开头
- 末尾有 `/` = 前缀匹配（目录）
- 末尾无 `/` = 精确匹配（文件）

示例：

```json
{
  "cache": {
    "excludePaths": [
      "/ubuntu/dists/",
      "/pypi/simple/",
      "/etc/resolv.conf"
    ]
  }
}
```

### 管理

查看统计：

```bash
scproxy --summary
```

清空缓存：

```bash
scproxy --clear-cache          # 交互式确认
scproxy --clear-cache --yes    # 跳过确认
```

响应头指示缓存状态：

- `X-Cache: HIT` — 来自缓存
- `X-Cache: MISS` — 从目标获取
- `X-Cache: BYPASS` — 未缓存（被策略排除）

### 存储结构

```
cache/
├── scproxy.json              # 配置文件
└── data/
    ├── github.com/
    │   └── releases/
    │       └── file.tar.gz
    └── api.example.com/
        └── endpoint.json
```

---

## 开发

```bash
make build          # 构建当前平台
make build-all      # 构建所有平台
make test           # 运行测试
make lint           # 运行代码检查
```

---

## 许可证

[MIT](LICENSE)
