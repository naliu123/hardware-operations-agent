# 硬件运维智能助手规格入口

已确认的业务需求见 [总体设计](../../docs/design/hardware-operations-agent-overall-design.md)，
实现约定见 [Go + Eino 技术设计](../../docs/design/hardware-operations-agent-technical-design.md)。

交付顺序与阶段见 [实施计划](../../docs/design/hardware-operations-agent-implementation-plan.md)。
独立工单位于 `issues/`，从 `01-qa-01.md` 开始，按依赖推进。

首项验证通过 HTTP 入口覆盖知识提交、发布、提问、结果读取和引用原文读取。
外部模型调用单独验证取消、失败和结构化输出。持久化采用临时真实存储验证重开后读取。
确定性模型和测试资料标记为 REPLAY，真实模型效果与实际设备接入单独验收。
