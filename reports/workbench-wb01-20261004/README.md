# WB-01 验收记录

日期：2026-10-04。结论：账号、PostgreSQL 私有归属与最小浏览器知识问答完成。
本记录不表示 WB-02～WB-08 的历史管理、附件、真实流式或 Python 沙箱已完成。

## 真实 PostgreSQL + REPLAY

使用 PostgreSQL 17.11、本机隔离临时集群、每项 WB 测试独立临时数据库。
测试结束删除数据库。问答通过公开 HTTP、外部 HTTP 模型 Adapter 与实际 Eino 图。
确定性模型结果标记 REPLAY，不以文件库或内存桩替代数据库。

- 两普通用户和一管理员完成登录、退出、创建账号、停用/启用、密码重置。
- 校验 Cookie 安全属性、Origin/CSRF、统一认证错误、持久化登录限制与最后管理员保护。
- 替换会话、响应、事件 ID 和幂等键不能越权；管理员不能读取他人私有问答。
- 伪造 owner 被拒绝；共享 Bearer token 被拒绝；知识和设备写入限管理员；
  原诊断入口在团队模式关闭。
- 密码重置和停用撤销旧登录；已建立的 SSE 在账号停用后主动关闭。
- 两用户分别完成知识问答；应用和数据库连接池重开后，登录会话和回答保持一致。
- 旧 001–004 数据库实际升级到 005；旧历史默认不可见，显式映射后可读，
  映射幂等；真实列约束发生漂移时升级失败且原数据保留。
- 默认追踪通过实际模型/知识流程验证，私有输入输出和密码未进入导出的 span。

证据：[数据库流程](hwops-wb-pg-tests.log)、[凭据与 SSE 撤销](hwops-wb-revocation.log)。
实现验收位于 `test/acceptance/workbench_identity_test.go`。

## 真实模型

真实 `deepseek-v4.1-flash` 通过用户名登录、真实临时 PostgreSQL、公开 HTTP、
原生知识工具调用和最终引用完成合成手册问答。

| 指标 | 结果 |
| --- | --- |
| 最终状态 | ANSWERED / LIVE |
| 主模型调用 | 2 |
| Prompt / Completion / Total tokens | 2959 / 137 / 3096 |
| 用时 | 8.90 秒（含初始化与登录） |
| 资料范围 | 合成蓝灯手册；不代表真实运维质量 |

[真实模型日志](hwops-wb-live.log)。本阶段尚未验证上游真实 stream 和视觉输入。

## 浏览器与工程检查

Playwright 使用独立 Chrome 上下文，Go 同域提供生产构建，数据库与外部 REPLAY
模型仍为上述实际运行链路。完成管理员创建账号 → 退出 → 普通用户登录 →
发送问题 → 确认 REPLAY 答案 → 打开定位到行号的原文。验证 Secure/HttpOnly Cookie、
普通用户无账号管理入口、localStorage 无凭据、无页面运行异常。

检查 1440×1000 桌面与 390×844 窄屏，窄屏没有横向溢出。

- [登录页](wb01-login.png)
- [桌面聊天](wb01-chat-desktop.png)
- [引用原文](wb01-citation.png)
- [窄屏聊天](wb01-chat-mobile.png)
- [浏览器测试](hwops-wb-e2e.log)：1 passed，2.5 秒。
- [Go 全量 race](hwops-wb-race.log)：通过，包含真实 PostgreSQL 的原问答/诊断回归
  与新工作台测试；acceptance 81.474 秒。
- `go vet ./...`、`go build ./...`、`git diff --check` 通过。
- [TypeScript 检查与 Vite 构建](hwops-web-build-node22.log)通过；Node 22.23.3，
  Vite 7.3.6。npm 安装审计为 0 vulnerabilities。

账号引导、迁移/备份回退和测试复现见[开发说明](../../docs/workbench-development.md)。
