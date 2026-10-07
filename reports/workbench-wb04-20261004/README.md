# WB-04 验收记录

日期：2026-10-04。私有图片、文字 PDF 与 UTF-8 日志的上传、持久解析、预览、
消息绑定和来源读取完成。完整模型 Agent 自动选择附件与 Python 仍由 WB-06 接入；
本报告不把现有知识问答输出描述为附件问答验收。

## 行为与实现

- 迁移 008 增加附件、逐页结果、派生图片、解析任务和消息附件关系。消息提交事务
  同时校验 owner、会话、解析终态、重复 ID、5 个文件和 100,000,000 字节上限。
- multipart 按流读取，不信任文件名、扩展名、声明 MIME 或 `Content-Length`。
  单文件严格限制 50,000,000 字节；原件完整后计算 SHA256 并以不可变键保存。
- 支持 PNG、JPEG、WebP、PDF 与严格 UTF-8 日志。图片限制 40,000,000 展开像素；
  日志保留原始行号、字节范围与内容哈希，页面读取不经模型改写。
- PDF 使用独立固定 parser 镜像逐页提取文本、表格和内嵌图片，并生成有界预览。
  201 页文件明确 UNSUPPORTED，不截取；纯扫描明确不支持 OCR，混合页为 PARTIAL，
  成功页与缺页原因分别保存。
- parser 结果以一个有界 ZIP 返回。应用拒绝路径穿越、重复路径、超 601 条目、
  超 100 MB 展开量、媒体签名、尺寸、字节数或哈希不一致，再保存派生对象。
- 原件、Range、页面、表格和派生图片读取均同时检查 owner、会话和未删除状态。
  管理员身份不提供他人私有附件读取权限；同一用户的其他会话也不能复用附件 ID。
- 删除会话先立即禁访，再取消 parser 身份、忘记 runner 产物、删除原件和派生文件。
  上传中断不建立附件；迟到 parser 结果不能重新公开或复活记录。
- 浏览器实现文件选择、剪贴板图片、拖拽 PDF/日志、真实上传百分比、解析状态、
  失败/部分缺口、原图、PDF 页、表格、内嵌图片和日志行预览。

## 实际解析环境

| 项目 | 实际值 |
| --- | --- |
| Linux | ARM64，Linux 6.8.0-117-generic |
| 隔离 | gVisor `20260928.0` / systrap / `hwops-runsc` |
| parser 镜像 | `sha256:7d4c4d72cc634fc365843c1378eeb0a30e18b7e23038517a1ca35a0d3cf03a61` |
| parser 包 | PyMuPDF 1.26.4、Pillow 11.2.1 |
| 探针哈希 | `631f8f48abc650e713ec9a88fceee3a407087a8d64bc256f1bee016b6cc60e8f` |
| 资源策略 | 1 CPU、1 GiB、60 秒、64 进程、256 MiB 临时目录、无网络 |
| PostgreSQL | 17.11；每项验收创建并删除独立数据库 |

实际公开 HTTP 流程通过：PNG 解码；日志行号；两页混合 PDF 的文字、矢量表格、
内嵌图片和页预览；纯扫描件 UNSUPPORTED；混合件 PARTIAL；损坏件 FAILED；
201 页文件 UNSUPPORTED。parser 使用与用户 Python 不同的端点、令牌、根目录和镜像。

实际探针还发现并修复了 systemd `UMask=0077` 对 runner 暂存输入权限的影响：
runner 现在写入后显式将仅供容器读取的代码和输入设为 `0444`、输入目录设为
`0555`；runner 根目录和 journal 继续保持私有。

## 限制与隔离

公开 HTTP 验证了 50,000,000 字节成功、50,000,001 字节拒绝；
100,000,000 字节消息成功、再增加文件拒绝；5 个附件成功、6 个拒绝。伪造
`image/png` 的文本按真实字节识别为日志，非法 UTF-8 在建立记录前拒绝。

两个用户和管理员交换附件 ID、同一用户跨会话交换 ID，以及 metadata、页面、
派生图片、原件和 Range 路径均返回不泄露存在性的 404。解析超时映射为明确
`PARSER_TIME_LIMIT_EXCEEDED`。解析中删除验证了即时 404、runner 取消、原件删除和
迟到成功不复活。中断 multipart 验证数据库无附件记录。

## 视觉与浏览器

- **REPLAY 外部视觉 Adapter：通过。** 公开上传后，可信应用按 owner 重新读取原始
  PNG，复核长度和 SHA256，以内联 data URL 交给外部多模态 Adapter。桩收到的解码
  字节与不可变原件逐字节一致；任意外部图片 URL 被 Adapter 拒绝。
- **真实视觉模型：NOT RUN。** 当前环境未配置真实视觉模型端点、模型名和密钥。
  这不影响实际 parser 验收，但不能据此标记 AC14 的真实视觉项通过。
- **浏览器：通过。** Playwright/Chrome 实际触发选择、paste 和 drop，检查上传/
  解析状态、消息附件持久化、日志行源和 PDF 表格/图片抽屉。截图：
  `web/test-results/wb04-attachment-drafts.png`、
  `web/test-results/wb04-pdf-source.png`。

完整模型多轮使用、附件引用进入最终答案以及模型自动调用 Python 在 WB-05/WB-06
复验。确定性 parser/视觉桩均标记 REPLAY；PostgreSQL、文件目录、Linux、gVisor
和固定 parser 镜像为 ACTUAL。

## 回归结果

- `go test -race ./... -count=1`：通过；接受测试 314.993 秒，包含实际 Python runner
  与 parser runner。
- `go vet ./...`、`go build ./...`、`git diff --check`：通过。
- `npm run build`：通过；生产 JS 370.10 kB（gzip 115.54 kB）。
- Playwright Chrome：2/2 通过，包含原有多会话流程和 WB-04 选择/paste/drop/预览。
