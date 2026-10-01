# 本地工单

本项目使用本地 Markdown，不向外部平台发布工单。

- 规格入口：`.scratch/hardware-operations-agent/spec.md`，指向已有总体及技术设计。
- 实施工单：`.scratch/hardware-operations-agent/issues/<NN>-<task-id>.md`，一项任务一份文件。
- 每份文件保留任务编号、交付行为、`Blocked by`、`Status` 和验收条件。
- `Status` 使用 `ready-for-agent`、`in-progress`、`done` 或 `needs-info`。
- 前置任务为 `done` 后才能开始依赖任务；真实接入验收缺口在任务中单独记录。
- 完成任务时更新证据和状态，并同步实施任务文档中的状态。

角色标签映射见 [triage-labels.md](triage-labels.md)。用户已经确定的任务无需重新进行需求分类。
