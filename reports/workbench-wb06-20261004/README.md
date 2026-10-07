# WB-06 验收记录

日期：2026-10-04。模型自动读取私有附件、调用隔离 Python、展示实际代码/日志/产物，
以及后续轮次把前轮产物作为新进程输入的聊天闭环已完成。

## 行为与实现

- 主 Agent 新增 `read_private_attachment` 与 `run_python_analysis`。模型参数只包含
  附件/产物 ID、读取范围、视觉问题、代码和 input IDs。
- owner、会话、响应、截止时间、固定镜像、1 CPU / 1 GiB / 60 秒、进程/磁盘/
  日志/产物限制、宿主路径与无网络策略均由可信应用和 runner 注入，不接受模型覆盖。
- 工具参数使用拒绝未知字段的严格 JSON 解码。伪造 owner、host path、network、
  image 或 CPU 配置会使模型输出无效，且不建立执行记录。
- 附件页、日志行、图片视觉结果与 Python 成功结果生成稳定 `source_id`。最终 claim
  只能原样引用本轮工具实际返回的 source；定稿前重新核对附件 owner/会话/哈希/
  状态，或执行 ID、代码、输入、stdout/stderr、产物哈希和 SUCCEEDED 状态。
- Python 每轮最多三次，失败和自动修正均由 PostgreSQL 原子计数；第四次返回稳定
  REJECTED，不绕过总时限。模型一次返回多个完整工具调用时按顺序串行执行。
- `tool_progress` 事件持久化 STARTING/RUNNING/终态。状态推进竞态通过重新读取当前
  版本和 CAS 重试处理，事件不会倒退。
- Response 保存实际执行代码、输入 manifest、受限 stdout/stderr、状态和产物。
  完整执行仍由私有执行 API 读取。取消、超时和重启在工具返回前发生时也会从数据库
  回填执行卡；重启不重跑模型或 Python。
- 会话历史程序化保留 execution/artifact/source 身份。后续模型只能使用本会话历史
  中列出的 artifact ID；每次执行都是新容器进程，不保留前轮变量。
- 浏览器实时显示执行状态、代码、标准输出/错误、稳定错误码、图片预览和文件下载。
  HTML/SVG 等主动格式仍强制下载。
- 附件与已发布知识冲突时，conflict 同时保留实际 fragment ID 与私有 source ID，
  页面明确显示差异，不自行选边。

## ACTUAL + REPLAY

公开 HTTPS、真实 PostgreSQL、外部确定性模型 Adapter、实际附件 parser 与实际
Linux/gVisor Python runner 完成：

1. 上传 CSV 日志，模型读取授权行并自动调用 Python。
2. pandas 按类别统计，matplotlib 生成 PNG，同时输出 CSV。
3. 浏览器和公开 API 展示实际代码、RUNNING/SUCCEEDED、stdout、CSV 下载与 PNG。
4. 断开 SSE 后任务继续，按事件序号补齐终态。
5. 下一轮从历史取得前轮 CSV artifact ID，在新进程中重新排序并生成新 CSV/PNG。
6. 同用户跨会话 artifact ID 返回 REJECTED；伪造可信参数不执行。
7. 三次失败建立三条实际执行记录，第四次被数据库预算拒绝。
8. 停止响应实际取消运行中的 gVisor 容器，清理完成后响应和执行均为 CANCELED。
9. 生成中删除、服务重启、队列并发和迟到结果沿用 WB-02/WB-03 实际验收并回归。
10. 图片原始字节和实际文字 PDF 页经受控工具读取并由 `source_id` 定稿；视觉模型
    使用外部 REPLAY Adapter，未声称真实视觉模型通过。

实际环境：

| 项目 | 结果 |
| --- | --- |
| Python 隔离 | Linux 6.8.0-117-generic / gVisor 20260928.0 |
| Python 镜像 | `sha256:8ca31a2511a3d13428b7d10b5d181f9edb90da9dcf0d25a95e9cf80252511572` |
| parser 镜像 | `sha256:7d4c4d72cc634fc365843c1378eeb0a30e18b7e23038517a1ca35a0d3cf03a61` |
| Python 包 | numpy 2.2.6、pandas 2.2.3、matplotlib 3.10.3、Pillow 11.2.1 |

## 真实模型

`deepseek-v4.1-flash` 使用公开登录、真实临时 PostgreSQL、真实上传日志与实际
gVisor runner，自动调用 Python 并生成 `summary.csv` 和 `duration.png`：

| 指标 | 结果 |
| --- | --- |
| 最终状态 | ANSWERED / LIVE |
| 模型调用 | 3 |
| Prompt / Completion / Total tokens | 9187 / 1272 / 10459 |
| Python | SUCCEEDED |
| 产物 | 2 |

这是小样本协议与隔离执行烟测，不代表真实运维分析质量。真实视觉模型仍为
**NOT RUN**，留到 WB-08 的完整真实接入矩阵。

## 浏览器

Playwright/Chrome 通过 3/3：

- 账号、多会话、停止、删除、重连与 WB-05 实时草稿；
- 图片/PDF/日志附件选择、paste、drop、预览和来源；
- 自动 Python 的状态、代码、stdout、CSV、PNG 预览与下载。

截图：[实际 Python 与产物](wb06-python-artifacts.png)。

## 回归结果

- `go test ./... -count=1`：通过。
- `go vet ./...`、`go build ./...`、`git diff --check`：通过。
- `npm run build`：通过；生产 JS 373.54 kB（gzip 116.51 kB）。
- Playwright Chrome：3/3 通过。
- `go test -race ./... -count=1`：通过；acceptance 379.136 秒，包含实际
  PostgreSQL、Linux/gVisor Python runner 与独立 parser runner。
