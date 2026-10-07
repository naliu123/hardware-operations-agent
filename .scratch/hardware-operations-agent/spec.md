# 硬件运维智能助手规格入口

已确认的业务需求见 [总体设计](../../docs/design/hardware-operations-agent-overall-design.md)，
实现约定见 [Go + Eino 技术设计](../../docs/design/hardware-operations-agent-technical-design.md)。

交付顺序与阶段见 [实施计划](../../docs/design/hardware-operations-agent-implementation-plan.md)。
独立工单位于 `issues/`，从 `01-qa-01.md` 开始，按依赖推进。

2026-10-04新增[多用户聊天工作台规格](../../docs/design/chat-workbench-spec.md)和
[技术方案](../../docs/design/chat-workbench-technical-design.md)，完整文档已获用户确认并
授权实现，WB-01～WB-08及AC01～AC14已完成。工单为 `15-wb-01-identity.md` 至
`22-wb-08-integration.md`，综合证据见 `reports/workbench-wb08-20261004/README.md`。

2026-10-07补充 WB-09 会话回复和 WB-10 默认思考区，均已完成。
思考与正文独立展示，生成中展开、完成后折叠，支持持久化和刷新恢复。
验收见[WB-10 报告](../../reports/workbench-wb10-20261007/README.md)。

首项验证通过 HTTP 入口覆盖知识提交、发布、提问、结果读取和引用原文读取。
外部模型调用单独验证取消、失败和结构化输出。持久化采用临时真实存储验证重开后读取。
确定性模型和测试资料标记为 REPLAY，真实模型效果与实际设备接入单独验收。
