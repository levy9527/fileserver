# AGENTS.md

## Project

**File Server** — 单文件内网文件浏览下载服务，Go 单二进制，无外部依赖。

## Build

```bash
go build -o fileserver .
```

## Cross-compile

```bash
GOOS=linux GOARCH=amd64 go build -o fileserver-linux .
GOOS=windows GOARCH=amd64 go build -o fileserver.exe .
```

## Usage

```bash
./fileserver                     # 默认端口 8080，服务当前目录
./fileserver --port 9000         # 指定端口
./fileserver --dir /path/to/dir  # 指定服务目录
```

## Architecture

```
main.go         # 入口：参数解析、HTTP 路由、启动打印
template.html   # 内嵌的 HTML 模板（go:embed 编译进二进制）
```

- `go:embed template.html` 将模板编译进二进制，部署只需一个文件
- 文件下载通过 `/dl/` 路径，使用 `http.StripPrefix` + `http.FileServer` 实现
- 目录浏览通过 `/` 路径，`os.ReadDir` 读取后渲染 HTML
- `filepath.Clean` + 前缀检测防止目录穿越攻击

## Key Decisions

- 无认证，内网直接访问
- 无前端框架，纯 HTML/CSS，内嵌编译
- 单文件二进制，零依赖部署
- 目录排序：文件夹在前，之后按字母排序
- 下载路径 `/dl/` 使用 `http.StripPrefix`，跟浏览路径 `/` 分开，避免冲突
