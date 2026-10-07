# WB-03 验收记录

日期：2026-10-04。持久化 Python 执行身份、独立 Linux runner、实际 gVisor 隔离、
资源限制、取消和重启回收完成。模型自动选择 Python 工具由 WB-06 接入；本报告验证
生产 Execution Interface 和公开执行/产物读取，不将外部模型桩描述为自动分析验收。

## 行为与实现

- 迁移 007 增加 `python_executions` 和 `artifacts`。执行先在 PostgreSQL 固定 owner、
  会话、响应、代码/请求哈希、输入 manifest、镜像摘要、预算和截止时间，再派发。
- runner 对执行 ID 幂等：同 ID 同请求返回原记录，不同请求拒绝；派发响应不明时
  只查询/取消原 ID，不自动重发代码。取消先落库，实际进程组及容器回收后才释放额度。
- 应用单实例数据库锁与 runner 双重限制全局最多两个执行；队列上限 20，每轮最多
  三次，仍服从消息五分钟截止时间。第三次持久排队，第四次明确拒绝。
- 每次创建全新 gVisor 容器：非 root、只读基础镜像、capabilities 全删、
  `no-new-privileges`、无网络/IPC、只读授权输入、独立 256 MiB tmpfs 输出。
- 固定 gVisor `20260928.0` 和镜像摘要；numpy、pandas、matplotlib、Pillow 版本由
  lock 文件固定并在启动探测中实际 import。探测失败时 runner 不监听，应用不回退。
- stdout/stderr 合计 1 MiB；最多 20 个产物、合计 100 MB。容器退出并回收后才读取
  输出，拒绝符号链接、硬链接、特殊文件、越界路径、超数量和超字节。
- 产物再次计算 SHA256 后进入带总配额及保留空间的不可变私有目录。公开 HTTP 支持
  owner 鉴权和 Range；另一用户及管理员均不能读取私有执行或产物。
- 应用重启将执行记为中断并取消原 runner 身份，不重跑。runner 控制端失联时不伪造
  成功或取消；重启先核对 journal、停止遗留 systemd 执行组并确认 Docker 回收。
- 删除会话立即禁访；runner journal、产物文件和数据库执行身份全部清理后，
  原有 `cleanup_jobs` 才完成会话清理。

## 实际隔离环境

| 项目 | 实际值 |
| --- | --- |
| Linux | ARM64，Linux 6.8.0-117-generic，cgroup v2 |
| 虚拟化 | macOS Virtualization.Framework / Colima 专用 Linux VM |
| gVisor | runsc release-20260928.0，systrap，runtime `hwops-runsc` |
| Python 镜像 | `sha256:8ca31a2511a3d13428b7d10b5d181f9edb90da9dcf0d25a95e9cf80252511572` |
| Python 包 | numpy 2.2.6、pandas 2.2.3、matplotlib 3.10.3、Pillow 11.2.1 |
| PostgreSQL | 17.11；每项验收创建并删除独立数据库 |

启动探测实际确认 UID 65532、禁网、输入只读、基础镜像只读、Docker socket 和宿主
工程目录不可见、没有 `HWOPS_*`/云/Docker 环境变量、工作目录为 256 MiB。
探测结果保存哈希；应用从 runner capability 核对镜像、预算和依赖版本。

## 实际资源与产物

最终 cgroup 复核记录：

| 场景 | 结果 |
| --- | --- |
| CPU | 四个忙循环墙钟 4.214 秒，cgroup CPU 4.097 秒，未超过 1 CPU |
| 内存 | 分配 1.5 GiB 被 OOM 终止；`memory.peak=1,073,741,824` |
| 进程 | `pids.peak=64`、`pids.events max=1`，状态 `PROCESS_LIMIT_EXCEEDED` |
| 墙钟 | sleep 90 秒在 60.112 秒终止，状态 `TIME_LIMIT_EXCEEDED` |
| 临时空间 | 连续写入在 268,435,456 字节得到 ENOSPC |
| 日志 | 2 MiB 输出只保留 1,048,576 字节并终止 |
| 产物 | 符号链接、硬链接、21 个文件及 100,000,001 字节文件均拒绝 |

真实 pandas 统计得到 `TOTAL=10`，matplotlib 生成 PNG，同时生成 CSV；下一次全新
进程通过产物 ID 读取 CSV 并算得均值 3.3333。执行记录保存代码哈希，输入和两个
产物均保存字节数及 SHA256：PNG 为
`39bfa0325a14abd6e46294d4785cc68effd558112920f60ca88f7d524f8c05d8`（22,363
字节），CSV 为 `cfdc701b22acb8fc6cd0523298b9d7a3e8f0dba603394b904760f0cdd4b36a23`
（24 字节）。跨会话复用同一产物 ID 被拒绝。

三个并发请求实测最多两个 `RUNNING`、一个 `QUEUED`；停止响应后两个运行容器和
排队身份均成为已清理的 CANCELED。随后新执行立即成功，证明额度只在清理后释放。

## 故障与恢复

- 相同 runner ID 重复提交只得到同一请求哈希；更换代码得到 409。
- 先取消后迟到提交命中持久 tombstone，不会启动。
- 应用进程停止时 runner 中原容器继续由独立 systemd 硬时限约束；应用重开后将
  PostgreSQL 记录标为 INTERRUPTED，取消并回收同一容器，`started_at` 未变化。
- runner 控制服务停止后，客户端只得到 unavailable，不得到成功/失败假结果；
  控制服务重开后将原 ID 标为 INTERRUPTED 并确认容器回收。
- 未配置 runner 时，带宿主写文件代码的执行请求返回 isolation unavailable，
  宿主标记文件不存在。

## 证据

- `hwops-wb03-actual.log`：最终二进制的全部 WB-03 race 场景，140.434 秒。
- `hwops-wb03-cgroup.log`：最终 runner 二进制的 CPU、内存和进程 cgroup 数值。
- `hwops-wb03-race.log`：完整仓库 race 与真实 PostgreSQL/runner 回归。
- `hwops-wb03-vet.log`、`hwops-wb03-build.log`：静态检查和全量构建。

确定性外部模型仅用于保持响应处于可执行阶段，标记为 REPLAY；Python、PostgreSQL、
Linux、gVisor、cgroup、文件系统及取消/重启均为实际环境，不是协议桩。
部署和复验方式见[工作台开发说明](../../docs/workbench-development.md)。
