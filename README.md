# 📁 File Server

> Single-binary Go file server for LAN file sharing — zero dependencies, no install, run it and share.

一个单文件内网文件浏览下载服务。Go 单二进制、无外部依赖，**一条命令启动**，局域网内任何设备打开浏览器即可浏览、下载、打包下载文件。

## ✨ 功能

- **目录浏览**：以网页形式浏览整个目录树，文件夹在前、按字母排序
- **文件下载**：单个文件一键下载
- **文件夹打包**：整个目录实时打包为 zip 流式下载
- **留言板**：内嵌消息板，可查看访问者的 IP 与操作系统（点击内容一键复制）
- **响应式界面**：手机 / 平板 / 桌面均可使用
- **单文件部署**：HTML 模板通过 `go:embed` 编译进二进制，交付只需一个文件

## 🚀 快速开始

```bash
# 下载对应平台的二进制后，直接运行
./fileserver                     # 默认端口 8080，服务当前目录
./fileserver --port 9000         # 指定端口
./fileserver --dir /path/to/dir  # 指定服务目录
./fileserver 9000                # 位置参数也可指定端口
```

启动后终端会打印局域网访问地址：

```
  📁 File Server
  Serving: /Users/you/files
  Port:    8080

  ➜ http://10.112.21.244:8080
```

同一网络下（例如同一个 Wi-Fi）的设备，用浏览器打开上面的地址即可访问。

## 🔧 构建

要求 Go ≥ 1.21。

```bash
# -ldflags="-s -w" 去掉符号表和调试信息，缩小体积（约 -35%）
go build -ldflags="-s -w" -o fileserver .
```

交叉编译：

```bash
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o fileserver-linux .
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o fileserver.exe .
```

## 🛡️ 安全说明

> ⚠️ **仅限内网使用，无认证机制。** 请不要将本服务直接暴露到公网。

- 目录穿越防护：基于 `filepath.Clean` + 前缀检测，防止 `../` 越权读取
- 留言板存储文件位于用户配置目录（`os.UserConfigDir()`），独立于服务根目录，不会被 `/dl/` 下载泄露
- 留言删除基于 IP 校验，只能删除自己发布的消息

## 🏗️ 技术架构

```
main.go         # 入口：参数解析、HTTP 路由、启动打印
template.html   # 内嵌 HTML 模板（go:embed 编译进二进制）
```

- **下载** `/dl/`：`http.StripPrefix` + `http.FileServer`
- **浏览** `/`：`os.ReadDir` 读取后渲染 HTML
- **打包** `/zip/`：目录实时压缩为 zip 流式下载
- 纯标准库实现，零第三方依赖

## 📄 License

MIT
