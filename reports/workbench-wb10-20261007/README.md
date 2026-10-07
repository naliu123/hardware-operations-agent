# WB-10 默认思考与独立思考区验收

2026-10-07，用户要求：“默认开启思考，页面实现思考区展示”。

已完成：DeepSeek 默认 `thinking.type=enabled`，Adapter 保留原生流和 buffered
`reasoning_content`，工具循环完整回传。思考以独立增量和快照保存，
正文上方实时展示，生成时展开、完成时折叠，可手动切换，刷新无重复。
停止、断流、重启保留已经收到的内容；只有思考不能被判成成功回答。
思考有 512 KiB 回复上限，不进入正文、检索、后续模型历史或追踪。

## REPLAY 与真实存储

`go test -race ./test/acceptance ./test/contracts ./internal/einoflow
-run 'TestWB10|TestWorkbenchConversational|TestWB05|TestWB02|TestReasoning|TestDeepSeek|TestOpenAI'
-count=1 -v -timeout=180s`

使用真实临时 PostgreSQL，模拟外部模型端点，通过公开 HTTP/SSE 验证；
acceptance 包通过，154.821s。该筛选未匹配 einoflow 单测，其完整单测由全仓测试覆盖。

- 默认思考参数、buffered/原生思考分片、工具回传、1 MiB 模型响应上限。
- 思考先于正文、工具前后分段、数据库快照与终态一致、Last-Event-ID 重放。
- 断流、仅思考、取消、重启、512 KiB 上限；终态拒绝迟到增量。
- 跨用户读响应及 SSE 被拒绝；思考不进入会话检索、后续模型历史和追踪。
- WB-02 历史/摘要/隔离、WB-05 正文增量/撤回/原子终态、WB-09 会话回复回归通过。

日志：[wb10-replay.log](wb10-replay.log)。

## LIVE DeepSeek

`TestWB10LiveReasoningAndNativeTool` 使用真实
`deepseek-v4.1-flash`、OpenCode Go `/zen/go/v1/chat/completions`、真实临时 PostgreSQL
及公开 HTTP/SSE。知识问题使用明确标注的合成手册，验证真实模型与原生工具协议，
不将合成手册视为真实设备证据。

| 场景 | 状态 | 思考增量 / 字节 | 正文增量 | 知识工具调用 | 首次思考 / 总耗时 |
| --- | --- | --- | --- | --- | --- |
| 身份介绍 | ANSWERED | 6 / 65 | 30 | 0 | 2.002s / 2.802s |
| 查询合成手册 | ANSWERED | 40 / 465 | 16 | 1 | 1.001s / 4.602s |

两次均保留 `COMPLETED` 思考及独立完整正文；原生工具后续调用正常。
日志：[wb10-live.log](wb10-live.log)。

## 浏览器与本地预览

Chrome Playwright 使用真实临时 PostgreSQL、Go 同源页面及外部 REPLAY 模型，
`web/tests/reasoning.spec.ts` 通过（1/1，11.8s）：
实时思考、手动收起/展开、生成中刷新与 SSE 重放无重复、完成后折叠、
重开后恢复、停止后保留并标中断、Markdown 脚本与远程图片不执行。
页面错误和思考区外部资源请求均为 0。

- [生成中截图](wb10-thinking-running.png)
- [完成后手动展开截图](wb10-thinking-complete.png)
- [浏览器日志](wb10-browser.log)

持续预览已重建并重启：[http://localhost:8080](http://localhost:8080)。
LIVE 浏览器实测新会话 `NQ37DVSSAELUYXGLJZXW3T4EOJ`：独立正文完整，
思考默认折叠，手动展开可见；刷新并展开后思考仍为相同 48 个字符，
正文为 154 个字符，没有页面横向溢出。旧记录未保存过的思考不回填。

## 构建与回归

- `go test ./...` 通过；按需外部环境测试仍遵循 opt-in，真实数据库和模型覆盖如上。
- `go vet ./...`、`git diff --check`、Go 服务构建通过。
- `npm run build` 通过（TypeScript + Vite，1m25s）。
- 临时浏览器 fixture 已停止；持续预览保留运行。

日志：[全仓测试](wb10-all.log)、[vet](wb10-vet.log)、[前端构建](debug-build.log)。
