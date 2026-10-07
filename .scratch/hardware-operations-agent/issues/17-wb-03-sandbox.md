# 17: WB-03 持久化 Python 沙箱执行

**目标：** 一次获授权的 Python 执行具有可查询、可取消、可核对的真实运行记录。

**Blocked by:** 15 (WB-01)

**Status:** done

**依据：** [规格](../../../docs/design/chat-workbench-spec.md) R20～R23；
[技术方案](../../../docs/design/chat-workbench-technical-design.md)第 10 节；
[ADR-0002](../../../docs/adr/0002-isolated-durable-python-execution.md)。

## 交付行为

实现 Go Execution Interface 和独立 Linux runner，持久化代码、输入清单、
执行身份、预算、状态、输出与产物。先写执行身份再派发，runner 幂等受理。
使用全新 gVisor 执行环境、预装依赖、只读授权输入和受限输出目录。

单次 60 秒、1 CPU、1 GiB，全局最多两个用户执行。取消终止整个执行组；
重启核对并清理遗留任务，不自动重跑。隔离或资源限制不可验证时拒绝执行。
固定解析程序可复用隔离基础，但使用单独镜像和有界解析队列。

## 验收条件

- [x] 在实际 Linux + gVisor 中运行统计和 matplotlib 绘图，保存代码、输入与产物哈希。
- [x] 相同执行 ID 重复提交只启动一次；同 ID 不同输入拒绝，失联不伪造成功。
- [x] 实际验证禁网、宿主/跨会话文件不可见、无应用凭据与 Docker socket。
- [x] 实际验证 CPU、内存、时间、进程数、临时空间与输出限制，而非只检查配置字段。
- [x] 三个以上并发请求中最多两个实际运行；排队时可取消，额度在清理后释放。
- [x] 运行中停止、控制端重启和 runner 失联均留下确定记录，遗留进程有硬时限并可回收。
- [x] 产物接收拒绝路径穿越、链接越界和超量输出；不完整输出不标成功。
- [x] gVisor 不可用或隔离探测失败时明确拒绝，不执行宿主 Python。
- [x] 用户只能读取自己的执行记录和产物；通过生产 Interface 测试实际 runner。

## 证据与状态

2026-10-04 完成。迁移 007、Execution Interface、私有文件存储、独立 runner、
公开执行/产物读取和删除清理已接入。实际环境为 Linux 6.8.0 ARM64、gVisor
20260928.0、固定镜像摘要和 PostgreSQL 17.11；统计、matplotlib、隔离反例、
cgroup 硬限制、两执行并发、停止、应用/runner 重启及 owner 隔离均通过。

最终 WB-03 race 套件 140.434 秒；完整仓库 race（含既有 QA/诊断和真实 runner）
272.713 秒。证据见
[验收记录](../../../reports/workbench-wb03-20261004/README.md)。
确定性外部模型仅用于保持响应阶段并标为 REPLAY；Python/gVisor 不是桩。
模型自动调用由 WB-06 接入。
