# Code Review 结果

- 审查对象：commit `0579678`（feat: 支持文件夹打包(zip)下载）
- 状态：工作区干净，已通过构建并实测验证（空目录、`/etc` 符号链接、链接环场景）

## 发现

### 1. 空目录会被静默丢弃 — 中

`main.go:288-301` 的 `zipDir` 只为文件创建 zip 条目，目录只做递归从不写自身条目。实测下载含空子目录的文件夹后，空目录在解压结果中完全消失（`ziptest/emptydir` 未出现在 zip 里）。对文件服务器这类场景，用户下载项目文件夹时空目录（如 `logs/`、`build/`）丢失是真实的困扰。

修复方式：递归前用 `header.Name = zipPath + "/"` 写入目录条目。

### 2. 符号链接目录会逃逸出服务根目录 — 低~中

`main.go:283` 的 `os.Stat` 跟随符号链接，递归会走入链接指向的目录。实测 `ln -s /etc dir` 后请求 `/zip/`，`/etc` 下的文件被直接打进 zip（`ziptest/evil-link/afpovertcp.cfg` 等）。这与既有 `/dl/` 行为一致（`http.FileServer` 也跟随链接，且本工具无认证、目录内容由运维自己放置），所以不算新攻击面；但 zip 是在**一次请求里批量递归**外部目录，且遇到链接环（`a -> .`）会递归到 `ENAMETOOLONG` 才终止。实测不会崩进程（路径增长被 PATH_MAX 限制），但返回的是一个 22 字节的空 zip。

### 3. 打包中途出错时静默返回损坏 zip — 低~中

`main.go:357-360`：`zipDir` 出错后直接 `return`，错误既不打日志也不给客户端任何信号；deferred 的 `zw.Close()` 失败也被忽略。客户端拿到的是一份截断的、无法解压的 zip。

建议至少 `fmt.Fprintf(os.Stderr, ...)` 记录，便于排查。

### 4. `Content-Disposition` 文件名未做 RFC 5987 编码 — 低

`main.go:353` 对中文/非 ASCII 目录名直接拼接，部分客户端下载时文件名会乱码。建议补充 `filename*=UTF-8''...`。与 `dirHandler`（main.go:162）既有模式一致，属历史遗留，但新代码可顺手修正。

### 5. 细节/外观 — 低

- `template.html:80` 的 `下载` 是 `<a>`，CSS 里没有对链接取消下划线（`.row` 的 `text-decoration:none` 不作用于子元素），按钮会带下划线；文件行按钮同样受影响。
- `template.html:20-21` 的 `.row.dir`、`.row.dir .name` 规则已无对应元素，属死代码（新增 `.dir-name` 已覆盖）。
- `/zip/文件名` 单文件路径会把扩展名剥离（`main.go:349`），zip 内文件变成无扩展名——该路径 UI 未暴露，属未使用分支，可忽略。

## 优先级

第 1、3 条值得优先修。
