# MCP 文件工具

> 状态：已实现
> 范围：strata MCP 工具层（`pkg/mcp`）
> 参考实现：rpi `packages/agent/src/harness/tools/edit.ts` 及其 Rust 移植 `rpi-tools/src/tools/edit.rs`

## 1. 背景

strata 的 MCP 接口提供 `exec`、`read`、`write`、`edit` 四个工具。文件工具直接操作会话持久
工作区，避免通过 `sed` / heredoc / 重定向修改文件时出现转义、非原子写入和格式变化：

- 不可预测：`sed` 换行、转义、缩进稍有不符就静默改错位置；
- 不可审计：改动过程无结构化记录，无法给出 diff；
- 不原子：写入到一半失败会留下半成品文件；
- 破坏格式：容易把 CRLF 变成 LF、丢掉 BOM。

本需求定义一个结构化文件编辑工具 `edit`：唯一语义是**精确文本替换**，由工具负责匹配唯一性、
区间不重叠、全量校验后原子落盘、格式原样保留。它的定位与 `read` / `write` 类工具互补：
`write` 负责整文件覆盖，`edit` 负责局部定点修改。

## 1.1 工作区与通用限制

- `read`、`write`、`edit` 操作会话持久工作区，即沙箱中的 `/root`。
- 相对路径从工作区根解析；绝对路径只允许 `/root` 及其后代。
- `..`、绝对符号链接和解析后越过工作区的符号链接均被拒绝。工作区内的相对符号链接按目标
  文件操作，链接本身保持不变。
- 仅支持普通文件。目录、设备、FIFO 等返回非文件错误。
- 仅支持合法 UTF-8 文本，拒绝内嵌 NUL/二进制内容和超过 4 MiB 的文件。
- `read` 返回原始可复用文本，不插入行号；`write` 与 `edit` 通过同目录临时文件、`fsync`
  和原子 rename 落盘。
- 同一会话内，针对同一规范路径的 `write` 和 `edit` 共用路径锁；不同路径可并行。
- MCP 身份仍使用 `owner_id` / `session_id` 参数或请求头，并通过 `manager.GetOrCreate`
  自动创建会话。

### `read`

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `path` | string | 是 | 工作区内的文件路径 |
| `start_line` | integer | 否 | 1 起的首行 |
| `line_count` | integer | 否 | 返回行数；只提供该参数时从第 1 行开始 |

结构化结果包含 `path`、`has_bom`、`line_ending`、`start_line`、`end_line` 和
`total_lines`。文件正文作为工具文本内容返回；BOM 从正文中剥离并通过 `has_bom` 标记。

### `write`

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `path` | string | 是 | 工作区内的文件路径 |
| `content` | string | 是 | 完整 UTF-8 文件内容；空字符串有效 |

`write` 可创建文件或覆盖已有文件，但不创建父目录。覆盖时保留原文件权限；新文件使用
`0644` 并由进程 umask 调整。结构化结果包含 `created`、`changed`、`bytes_written`；
覆盖已有文本且内容变化时还包含 `diff`、`patch` 和 `first_changed_line`。

### `edit`

## 2. 目标与非目标

**目标**

- 单文件、一次调用可含多处定向替换，结果可预测；
- 匹配失败、重复匹配、编辑区间重叠一律显式报错，禁止"猜一个位置"；
- 全部编辑校验通过后才写盘，失败时文件不变；
- 保留原文件的 BOM 与行尾风格（CRLF / LF）；
- 返回可展示给用户的 diff 与 unified patch。

**非目标**

- 不做全文重写（属 `write` 类工具）；
- 不做正则 / 通配替换；
- 不做格式化、lint、编码转换；
- 不支持二进制文件；
- 不做多文件批量编辑（一次调用只针对一个 `path`）；
- 不提供 HTTP 端点，只作为 MCP 工具存在（`exec` 的 HTTP 接口不随之扩展）。

## 3. 工具契约

### 3.1 注册与身份

- 工具名：`edit`，注册在 `pkg/mcp/tools.go` 的 `SetupTools`，与 `exec` 同级；handler 实现
  位于 `pkg/mcp/file_edit.go`（`handleEdit`）。
- 身份解析复用 `identity.ParseScarf(ctx, identity.FromArgs(args))`，即 `owner_id` /
  `session_id` 可来自调用参数或请求头，语义与 `exec` 完全一致。
- 会话获取方式与 `exec` 一致（`manager.GetOrCreate`），不引入"会话必须已存在"的特例。

### 3.2 输入参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `path` | string | 是 | 目标文件路径，按**沙箱内视角**解释（见 R2） |
| `edits` | array | 是 | 至少 1 个替换项；空数组为非法输入 |
| `edits[].oldText` | string | 是 | 被替换的精确原文，不可为空，必须在原文中唯一出现 |
| `edits[].newText` | string | 是 | 替换文本，允许为空字符串（等价于删除） |

字段名固定为 **camelCase**（与 rpi 及主流供应商工具调用约定一致），不接受 `snake_case` 别名。

为兼容历史调用方，以下三种写法**应当**被内部规范化后接受（实现层转换，不作为文档推荐用法）：

1. 顶层直接给 `oldText` / `newText`，无 `edits`；
2. `edits` 传成 JSON 字符串而非数组；
3. `edits` 传成单个对象而非数组。

### 3.3 返回值

成功时返回文本与结构化 details：

- 文本：`Successfully replaced {N} block(s) in {path}.`
- `details.diff`：带 4 行上下文的行级 diff；
- `details.patch`：unified patch，含 `---`/`+++` 文件头，**不含时间戳**；
- `details.first_changed_line`：首个变更行号（1 起，可选字段）。

失败时返回工具错误（`mcp.NewToolResultError`），消息文本见 §5。

## 4. 行为规范

**R1 参数校验**
`path` 非空；`edits` 至少 1 项；每个 `oldText` 非空。任一不满足立即报错，不触碰文件系统。

**R2 路径解析（会话内视角）**
`path` 以沙箱内根 `/` 为基准，相对路径相对会话工作目录（`/root`，即会话 `home`）。
工具只按「会话根 + `path`」定位文件，**不感知 overlay 驱动**（`fuse` / `kernel` / `none` 的差异由
sandbox 层负责，会话根下看到什么就改什么），也不得通过 `exec` 拼 shell 命令完成读写。

必须拒绝两类越界：规范化 `..` 后逃逸出会话根；解析后指向会话根之外的符号链接。

**R3 目标类型**
`path` 必须是普通文件或指向文件的符号链接。目录、设备、FIFO 等返回
`Could not edit file: {path}. Path is not a file.`

**R4 快照一致性**
所有 `edits` 都针对**同一份原始内容**定位，不允许"上一个 edit 改完再找下一个"。因此相邻或同块
的多处改动必须合并进一个 edit 项。

**R5 精确匹配优先**
先在原始内容上做逐字节精确查找（含全部空白与换行）。

**R6 宽松匹配回退**
精确匹配失败时，允许一次归一化后再匹配。归一化规则限定为：

- Unicode NFKC 归一化；
- 每行去除**行尾**空白（不做行首缩进归一化）；
- 弯引号 `‘’‚‛ “”„‟` 归一为大写 ASCII 引号；
- 各类连字符 / 减号 / 破折号（`‐‑‒–—―−`）归一为 `-`；
- 各类不换行/窄空格（`U+00A0`、`U+2002..U+200A`、`U+202F`、`U+205F`、`U+3000`）归一为普通空格。

回退匹配只能用于定位；写入基准内容也必须是归一化后的视图，以保证替换位置与偏移自洽。

**R7 唯一性**
`oldText` 在（归一化后的）内容中必须**恰好出现 1 次**。出现 ≥2 次要报错，并在错误消息中附上最多
5 处匹配的行号诊断（行号仅用于辅助定位，提示调用方扩大 `oldText` 上下文）。

**R8 区间不重叠**
所有命中的区间按起点排序后必须两两不相交；重叠时报错并指出冲突的 `edits[i]` / `edits[j]`。

**R9 原子写入**
先完成 R1–R8 的全部校验，再一次性写回。任何一步失败都不允许留下部分修改。若替换后内容与原文
完全一致，返回"无变化"错误而不是成功。

**R10 格式保留**
写入前剥离的 BOM 必须原样写回；行尾风格按原文首个行尾探测结果统一恢复（原文 CRLF 则写回 CRLF）。

**R11 并发控制**
同一会话内对同一 `path` 的 `edit` 调用必须串行化（按路径加锁），避免读-改-写竞态；不同路径可并行。
不同会话之间互不影响。

**R12 取消与超时**
必须支持上下文取消；取消后不得写盘。单次调用受独立超时配置约束：新增环境变量
`STRATA_SERVER_EDIT_TIMEOUT`（默认 `30s`，与 `STRATA_SERVER_SHELL_TIMEOUT` 相互独立）。

**R13 文件大小上限**
必须设置读取上限以防超大文件打爆服务内存，超限返回明确错误，不做部分处理。阈值 **4 MiB**
（单文件，读入后的大小）。

## 5. 错误与消息

消息文本逐字对齐参考实现，便于 agent 复用既有提示词与自纠错流程。

| 场景 | 消息 |
|------|------|
| `oldText` 为空（单个） | `oldText must not be empty in {path}.` |
| `oldText` 为空（多个） | `edits[{i}].oldText must not be empty in {path}.` |
| 找不到（单个） | `Could not find the exact text in {path}. The old text must match exactly including all whitespace and newlines.` |
| 找不到（多个） | `Could not find edits[{i}] in {path}. The old text must match exactly including all whitespace and newlines.` |
| 不唯一（单个） | `Found {n} occurrences of the text in {path}. The text must be unique — include more surrounding context to disambiguate.` |
| 不唯一（多个） | `Found {n} occurrences of edits[{i}] in {path}. The text must be unique — include more surrounding context to disambiguate.` |
| 不唯一附加诊断 | `Matching locations (line labels are diagnostic; do not include them in oldText):` 后跟最多 5 处 `Match n at line L:` 与前后 1 行片段（单行超过 200 字符截断并标注 `[line truncated]`），末尾提示 `Read the intended region and include enough surrounding text to make oldText unique before retrying.` |
| 区间重叠 | `edits[{i}] and edits[{j}] overlap in {path}. Merge them into one edit or target disjoint regions.` |
| 无实际变化（单个） | `No changes made to {path}. The replacement produced identical content. This might indicate an issue with special characters or the text not existing as expected.` |
| 无实际变化（多个） | `No changes made to {path}. The replacements produced identical content.` |
| `edits` 为空 | `Edit tool input is invalid. edits must contain at least one replacement.` |
| 目标非文件 | `Could not edit file: {path}. Path is not a file.` |
| 路径越界 | 需新增：`Path escapes session root: {path}.` |

## 6. 验收测试

| 用例 | 预期 |
|------|------|
| 单处精确替换 | 文件内容正确，返回 diff/patch，`first_changed_line` 正确 |
| 同块多处改动合并为一个 edit | 成功 |
| 同一处改动拆成两个重叠 edit | 报 overlap 错误，文件不变 |
| `oldText` 在文件中出现 2 次 | 报不唯一错误，消息含匹配位置清单 |
| `oldText` 不存在 | 报 not found，文件不变 |
| `oldText` 为空 | 报空文本错误，文件不变 |
| `newText` 为空 | 等价删除该段，成功 |
| 替换后内容不变 | 报 no change 错误 |
| CRLF 文件 | 写回后仍为 CRLF |
| 带 BOM 的文件 | 写回后 BOM 保留 |
| 弯引号 / 不换行空格 / 行尾空格与原文不同 | 宽松匹配命中并替换成功 |
| 缩进不同 | 宽松匹配**不命中**，报 not found |
| 相对路径 `/root` 下文件 | 正确解析并修改 |
| `path` 含 `../` 逃逸会话根 | 报路径越界错误 |
| `path` 指向目录 | 报 not file 错误 |
| 两个并发 `edit` 改同一文件 | 串行执行，无内容丢失 |
| 调用中途取消 | 不写盘，文件保持原状 |
| 超过大小上限的文件 | 报明确错误，不改动 |

## 7. 开放问题

1. **是否需要 `unified patch` 省略**：小改动下 patch 体积可观，是否提供开关或按需返回。
2. **多文件编辑**：一次调用编辑多个文件（`edits[]` 每项带 `path`）留待后续需求评估。

## 7.1 已定决策

- 路径解析不区分 overlay 驱动，驱动差异由 sandbox 层承担。
- 不提供 HTTP 端点，仅 MCP 工具。
- 超时独立配置 `STRATA_SERVER_EDIT_TIMEOUT`，默认 `30s`。
- 单文件大小上限 4 MiB。
- `read` 使用 1 起算的可选 `start_line` / `line_count`；`write` 不创建父目录；三者共享
  UTF-8、4 MiB 和工作区边界。
- `edit` 单独受 `STRATA_SERVER_EDIT_TIMEOUT` 控制，默认 `30s`。

## 8. 参考实现要点

- 匹配、重叠检测、宽松归一化、diff/patch 生成：`rpi-tools/src/tools/edit_diff.rs`
- 工具入口、参数规范化、BOM/行尾处理、写盘：`rpi-tools/src/tools/edit.rs`
- 工具描述文本（供 agent 阅读的使用约定）：同文件 `description` 字段，要点为"先 `read` 再拷贝原文、
  不要带显示行号、`oldText` 保持唯一、同块改动合并、不要重叠"。
