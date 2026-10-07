# WB-08 团队部署与综合验收

日期：2026-10-04。多用户聊天工作台的部署资产、恢复演练、10 会话容量和
AC01～AC14 综合矩阵已完成代码环境验收。

## 结果边界

本报告严格区分：

- **REPLAY MODEL**：外部 HTTP 确定性模型 Adapter，用于可重复的容量与浏览器验收。
- **LIVE MODEL**：`deepseek-v4.1-flash` 的真实原生 tools、stream 和图片调用。
- **ACTUAL DATABASE/FILES**：PostgreSQL 17.11 与真实临时私有文件目录。
- **ACTUAL SANDBOX**：Linux 6.8.0-117-generic、gVisor 20260928.0 和固定镜像。

没有把 REPLAY 结果计作真实模型效果，也没有把 LIVE 模型输出计作现场设备状态。
本工作区没有团队生产域名、证书和目标应用主机，因此生产 Caddy/systemd 上线为
**NOT RUN**；交付的是可复现部署资产和在等价应用/runner 拓扑中的验收结果。

## 部署与恢复

新增：

- `deploy/app/hwopsd.service`：无特权单实例服务、只读系统、私有状态目录。
- `deploy/app/hwops.env.example`：PostgreSQL、同域入口、LIVE 模型、文件额度、
  runner/parser 和上下文预算清单。
- `deploy/app/Caddyfile`：HTTPS 同域反向代理及 SSE 立即 flush。
- `deploy/app/backup.sh`：停应用后同点导出 PostgreSQL custom dump、文件 tar 和 hash。
- `deploy/app/restore.sh`：显式确认、hash 校验、事务恢复、旧文件保留和失败停机。
- `docs/workbench-development.md`：构建、管理员引导、TLS、runner、升级、备份、
  恢复和故障处理。

`TestWB08PostgresDumpAndPrivateFilesRestore` 实际使用 PostgreSQL 17
`pg_dump/pg_restore`：

1. 通过公开 HTTP 创建会话并上传、解析私有日志。
2. 停止源应用，导出数据库并复制私有文件目录。
3. 恢复到全新数据库和文件目录。
4. 用恢复后的账号登录，通过公开 HTTP 读取同一会话、解析行和原始字节。

迁移 checksum、账号密码 hash、会话/附件身份和 SHA256 均保持一致。项目不提供
默认公共账号或密码，首次管理员仍需离线从标准输入引导。

## 10 会话容量

`TestWB08TenActiveConversationsTwoPythonExecutions` 通过公开 HTTPS 接口同时提交
10 个独立会话。每个会话由外部 REPLAY 模型原生调用一次实际 gVisor Python，
并通过 SSE 返回最终正文。

| 指标 | 结果 |
| --- | ---: |
| 受理 / 终态 ANSWERED | 10 / 10 |
| 失败 / 超时 | 0 / 0 |
| Python RUNNING 峰值 | 2 |
| Python STARTING + RUNNING 峰值 | 2 |
| QUEUED 峰值 | 8 |
| 实际进入排队的执行 | 8 |
| 排队等待 P50 / P95 | 3.777 s / 7.270 s |
| 首正文增量 P50 / P95 | 5.203 s / 8.604 s |
| 端到端 P50 / P95 | 5.203 s / 8.604 s |

应用 PostgreSQL 队列硬上限为 20，每轮 Python 硬上限为 3；runner 再次独立限制
最多 2 个任务。WB-03 另验证第四次调用拒绝、资源超限、总时限、停止和回收。
本次 10 会话容量使用 REPLAY 模型，未宣称真实模型 10 并发容量通过。

## 真实接入

| 项目 | 结果 | 证据 |
| --- | --- | --- |
| PostgreSQL | PASS | 17.11；迁移、重启、dump/restore 和私有文件恢复 |
| 原生知识工具 + stream | PASS | LIVE；21 个正文增量，3 次模型调用，4765/236/5001 tokens |
| 自动 Python tool-call | PASS | LIVE；实际 gVisor，2 个产物，9274/1119/10393 tokens |
| 图片字节读取 | PASS | LIVE；实际 PNG、私有 source_id，4675/552/5227 tokens |
| 附件 parser | PASS | Linux/gVisor 固定 parser 镜像，实际 PDF/图片/表格 |
| Python 隔离 | PASS | 网络、文件越权、CPU、内存、磁盘、输出、停止、重启回收 |

最终 LIVE 知识测试曾暴露模型把公开资料 `source` URI 错放进私有 `source_ids`。
服务始终拒绝该答案，没有降级为 REPLAY。修正提示现明确区分公开 `fragment_ids`
和私有工具 `source_ids`；确定性回归和真实模型复跑均通过。

## AC01～AC14

| 验收项 | 结果 | 主要证据 |
| --- | --- | --- |
| AC01 身份与隔离 | PASS | 两用户/管理员、ID 互换、SSE/附件/执行/产物越权反例 |
| AC02 持久化 | PASS | 刷新、跨浏览器、应用重启、PostgreSQL dump/restore |
| AC03 多轮上下文 | PASS | 历史摘要、附件与前轮产物、新进程追问、跨会话拒绝 |
| AC04 真实流式 | PASS | 外部模型多帧正文、LIVE 18 增量、内部 JSON 不外泄 |
| AC05 草稿复核 | PASS | 无效来源、撤回资料、设备变化、重连和最终原子提交 |
| AC06 附件 | PASS | paste/drop、图片、文字 PDF、表格、扫描缺口、201 页拒绝 |
| AC07 限制 | PASS | 数量/字节/页数、CPU/内存/时间/磁盘/输出/次数/总预算 |
| AC08 真实沙箱 | PASS | 实际 Linux + gVisor，禁网、只读输入、宿主与凭据隔离 |
| AC09 分析闭环 | PASS | 日志 → 自动 Python → CSV/PNG → 前轮产物追问 |
| AC10 停止恢复 | PASS | 断线续跑、实际停止、应用/runner 重启不重跑、旧记录保留 |
| AC11 删除 | PASS | 运行时立即禁访、迟到结果阻断、文件/产物清理重试 |
| AC12 界面 | PASS | Chrome 4/4、桌面/390 px、键盘、长内容、安全 Markdown |
| AC13 容量 | PASS | REPLAY 模型 + actual sandbox：10 终态、2 运行、8 排队 |
| AC14 真实接入 | PASS | PostgreSQL、LIVE tools/stream/vision、actual parser/sandbox |

## 回归

- `go test ./... -count=1`：通过；acceptance 166.355 秒。
- `go test -race ./... -count=1`：通过；acceptance 409.893 秒。
- `go vet ./...`、`go build ./...`、`git diff --check`：通过。
- `npm run build`：通过；生产 JS 376.78 kB（gzip 117.58 kB）。
- Playwright/Chrome：4/4 通过。
- PostgreSQL dump/restore：通过。
- 10 会话容量：通过。
- LIVE tools/stream、Python、vision：通过。
- `bash -n deploy/app/backup.sh deploy/app/restore.sh`：通过。
- Linux `systemd-analyze verify`：通过（替换目标机用户/二进制为验证占位）。
- Caddy 2.10 `caddy adapt`：通过。
