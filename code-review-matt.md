# 代码审查报告 — fileserver

- **审查对象**：`main` 分支（整个代码库）
- **基线（固定点）**：空树（`4b825dc…`），因此 diff 覆盖全部代码
- **提交列表**：`bc554ea → 657496d → ce836a9 → 8328121 → 0e2fdfe → 0579678`
- **日期**：2026-08-31

---

## Standards（编码规范维度）

依据仓库自身文档 `AGENTS.md` 的既定规范逐项核对，**未发现硬性违规**，以下文档化规范均已满足：

- ✅ `go:embed` 单文件二进制，零外部依赖（`go.mod` 无 require）
- ✅ `/dl/` 使用 `http.StripPrefix` + `http.FileServer`（`main.go:396`）
- ✅ 目录浏览使用 `os.ReadDir`（`main.go:178`）
- ✅ `filepath.Clean` + 前缀检测的目录穿越防护，`dirHandler`（165-169）与 `zipHandler`（284-288）均具备；zip-slip 安全——文件名基于 `filepath.Base` + 守卫下拼接的路径生成，无法触达 `..`
- ✅ 文件夹优先 + 字母排序（191-195）；无前端框架
- ✅ `go build`、`go vet`、`gofmt` 全部通过

### 发现项（均为主观判断，非硬性违规）

| # | 级别 | 位置 | 问题 |
|---|------|------|------|
| 1 | **中** | `main.go:396` | **`.messages.json`（含客户端 IP 与留言）可通过 `/dl/` 下载**。`http.FileServer` 会提供隐藏文件，访问 `/dl/.messages.json` 即可拿到原始数据存储；而 `/`（175）与 `/zip/`（294）都显式隐藏了该文件。隐藏逻辑不一致，在无鉴权的内网环境下造成数据泄露。 |
| 2 | 低 | `main.go:166` / `template.html:186` | **根目录出现无效的「返回上级目录」链接**。`rel="."` 使得 `{{if .Path}}` 渲染出一个无效链接（`../` 无操作），且标题栏显示 `/./`。 |
| 3 | 低 | `main.go:169, 288` | **穿越守卫误伤**。`strings.HasPrefix(rel, "..")` 会拦截合法名称，例如以 `..` 开头的目录（`..hidden`）无法访问。 |
| 4 | 低（代码异味） | `main.go:165-169` vs `284-288` | **重复代码**。`TrimPrefix`+`Clean`+`HasPrefix` 守卫块在两个 handler 中逐字重复；建议抽取 `safeRel(path, prefix)` 辅助函数。 |
| 5 | 低（代码异味） | `main.go:175, 294, 399` | **（潜在）霰弹式修改**。字符串 `".messages.json"` 在三处硬编码；改名需改 3 处，建议提升为 `const` 常量。 |
| 6 | 低 | `main.go:55, 61, 199` 等 | **忽略错误**。`json.Unmarshal`、`os.WriteFile`、`filepath.Abs`（×3）、`tmpl.Execute` 的错误被丢弃；JSON 损坏时静默重置，下次 `save()` 会覆盖掉原数据。 |
| 7 | 低 | `main.go:75` | **ID 冲突**。用 `time.Now().UnixMilli()` 作为留言 ID；同一毫秒内两条留言会冲突，删除时会同时删掉两条。 |
| 8 | 低 | `main.go:225-235` | **POST 无限制 / 无 CSRF**。无 Content-Length 或条数上限，跨站表单 POST 可注入留言；`Content-Disposition` 文件名（141、320）存在理论上的 CR/LF 注入。 |
| 9 | 低 | `AGENTS.md` | **文档漂移**。AGENTS.md 仅覆盖浏览功能，留言板与 `/zip/` 功能未在文档中说明。 |

---

## Spec（需求维度）

**无可用 Spec。** 提交信息中无 issue-tracker 引用，仓库内 `docs/`、`specs/`、`.scratch/` 下均无 PRD/Spec 文件，也未提供外部 Spec 路径。因此该维度按流程跳过。

---

## 总结

**Standards 维度**：共 9 项发现（1 项中危，其余为低危 / 判断项）；最严重的是 #1——`/dl/` 会公开返回包含客户端 IP 的原始 `.messages.json`。

**Spec 维度**：跳过（无 Spec 来源）。
