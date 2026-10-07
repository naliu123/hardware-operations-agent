# 聊天工作台开发与账号引导

当前交付范围以 [WB 工单](design/hardware-operations-agent-implementation-plan.md#8-多用户聊天工作台)
为准。WB-01～WB-08 提供账号、私有多会话问答、持久化历史、有界摘要、真实流式、
图片/PDF/日志附件，以及模型自动调用的独立 gVisor Python 执行。

## 运行

需要 Go 1.26、Node 22.12+、PostgreSQL。已验证 PostgreSQL 17.11、Node 22.23.3
和 26.9.0；依赖由 Go 和 npm 锁文件固定。前端构建：

```sh
cd web
npm ci
npm run build
```

应用从项目根目录运行。通过部署密钥配置设置 `HWOPS_DATABASE_URL` 与模型凭据，
不要将密码或连接凭据提交到版本库。启动前首次引导管理员，密码从标准输入读取：

```sh
go run ./cmd/hwops-admin -command bootstrap -username admin < /受限目录/初始密码文件
```

输入文件必须包含一行、以换行结尾的 12–72 字节密码。文件使用 `0600` 权限。
引导只允许空账号库，不提供默认账号或默认密码。该命令需获取应用单实例锁，
因此引导和历史映射在应用停止时运行。

```sh
export HWOPS_AUTH_MODE=users
unset HWOPS_API_TOKEN
export HWOPS_PUBLIC_ORIGIN=https://hwops.example.internal
export HWOPS_WEB_DIR=web/dist
export HWOPS_LISTEN_ADDR=127.0.0.1:8080
go run ./cmd/hwopsd
```

HTTPS 由同域入口终止并代理到 Go；`HWOPS_PUBLIC_ORIGIN` 必须精确包含协议和端口，
且不含路径。仅本机开发允许 `http://localhost:端口`，浏览器 Cookie 仍设置 Secure。
团队模式强制 PostgreSQL，拒绝共享令牌；`local_token` 仍是原 CLI/回放的独立默认模式。

## 团队单实例部署

本期只支持一个 `hwopsd` 应用实例。PostgreSQL advisory lock 会拒绝第二实例，不能
通过多副本绕过全局调度：应用最多同时处理 10 个回答，Python 持久队列最多 20 个，
应用和 runner 都强制最多 2 个 Python 处于启动或运行状态。水平扩容需重新设计全局
执行额度，不属于本期。

建议应用主机使用受支持的 Linux、PostgreSQL 17 客户端、Caddy 及 systemd；Python
runner/parser 仍部署到后文所述的专用 Linux 主机。发布构建在可信构建机完成：

```sh
cd web
npm ci
npm run build
cd ..
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o dist/hwopsd ./cmd/hwopsd
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o dist/hwops-admin ./cmd/hwops-admin
```

目标主机创建无登录用户并安装只读程序和前端；`GOARCH` 按目标主机调整：

```sh
sudo useradd --system --home /var/lib/hwops --shell /usr/sbin/nologin hwops
sudo install -d -o root -g root -m 0755 /opt/hwops/web
sudo install -d -o root -g hwops -m 0750 /etc/hwops
sudo install -d -o hwops -g hwops -m 0700 /var/lib/hwops/files
sudo install -m 0755 dist/hwopsd /usr/local/bin/hwopsd
sudo install -m 0750 dist/hwops-admin /usr/local/bin/hwops-admin
sudo rsync -a --delete web/dist/ /opt/hwops/web/dist/
```

创建独立 PostgreSQL database/role，强制 TLS；应用角色需拥有该数据库 schema 的
DDL/DML 权限，因为每次启动都核对迁移版本和 checksum，升级时在事务内应用新迁移。
测试和业务库不得共用：

```sh
sudo -u postgres createuser --pwprompt hwops_app
sudo -u postgres createdb --owner=hwops_app hwops
```

以 [`deploy/app/hwops.env.example`](../deploy/app/hwops.env.example) 为清单写入
`/etc/hwops/hwops.env`，替换所有示例值并设为 `root:hwops 0640`。runner token 文件
分别使用独立随机值、`root:hwops 0640`；模型和数据库凭据不得提交到仓库。模型密钥
含 shell 特殊字符时使用单引号，因为备份工具会读取这份 root-owned 配置。

```sh
sudo install -o root -g hwops -m 0640 deploy/app/hwops.env.example /etc/hwops/hwops.env
sudo install -m 0644 deploy/app/hwopsd.service /etc/systemd/system/hwopsd.service
sudo systemctl daemon-reload
```

首次启动前在应用停止时引导管理员。密码文件只保留一行、以换行结尾且权限为 0600，
命令成功后立即删除；项目没有公共默认账号或密码：

```sh
sudo systemctl stop hwopsd.service
sudo sh -c '. /etc/hwops/hwops.env
exec env -i PATH=/usr/local/bin:/usr/bin:/bin HWOPS_DATABASE_URL="$HWOPS_DATABASE_URL" \
  runuser -u hwops -- /usr/local/bin/hwops-admin -command bootstrap -username admin' \
  < /受限目录/初始密码文件
sudo rm -f /受限目录/初始密码文件
```

上例只向离线命令注入 `HWOPS_DATABASE_URL`，连接串不会进入命令历史。

HTTPS 示例位于 [`deploy/app/Caddyfile`](../deploy/app/Caddyfile)。替换域名和证书
路径，确保 Caddy 私钥只对代理用户可读；`flush_interval -1` 保留 SSE 增量。内部 CA
证书必须同时加入团队浏览器、应用主机及 runner 客户端系统信任，不能关闭 TLS 校验。

```sh
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl enable --now caddy hwopsd
curl --fail --silent https://hwops.example.internal/healthz
sudo journalctl -u hwopsd -n 100 --no-pager
```

启动会硬校验 PostgreSQL 迁移、前端构建目录、私有文件额度、模型上下文预算、runner
TLS/token、实际 gVisor capability、固定镜像和包版本。任一配置不一致时服务直接失败；
不回退到宿主 Python、普通容器、文件数据库或 REPLAY。配置 LIVE 模型失败时请求返回
`MODEL_UNAVAILABLE`，也不会自动改成 REPLAY。

## 独立 Python runner

runner 必须部署在专用 Linux 5.6+ 主机，使用 systemd、Docker、cgroup v2 和 gVisor；
不能与 macOS 应用进程或普通宿主 Python 混用。控制程序以 root 运行以创建 tmpfs、
核对 cgroup 并调用 Docker，但用户代码只在全新 gVisor 容器内以 UID 65532 运行。

仓库将 gVisor 固定为 `20260928.0`，安装脚本同时校验发布包内 SHA512 和仓库固定的
SHA256。先审核脚本，再在 runner 主机执行：

```sh
sudo deploy/runner/install-gvisor.sh
sudo docker build -t hwops-python:wb03 deploy/runner
sudo docker image inspect hwops-python:wb03 --format '{{.Id}}'
```

最后一条命令返回的 `sha256:...` 是该架构构建的不可变镜像身份。将它写入权限为
`0600` 的 `/etc/hwops/runner.env`：

```text
HWOPS_RUNNER_IMAGE_DIGEST=sha256:替换为实际值
HWOPS_RUNNER_LISTEN=0.0.0.0:8091
```

交叉编译并安装 runner；`GOARCH` 按主机选择 `arm64` 或 `amd64`：

```sh
GOOS=linux GOARCH=arm64 go build -o hwops-runner ./cmd/hwops-runner
sudo install -m 0755 hwops-runner /usr/local/bin/hwops-runner
sudo install -m 0644 deploy/runner/hwops-runner.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now hwops-runner
```

`/etc/hwops/runner-token` 使用独立随机值且权限为 `0600`；TLS 证书和私钥分别放在
`/etc/hwops/runner.crt`、`/etc/hwops/runner.key`，私钥为 `0600`。runner 对远程地址
监听时强制 TLS；只允许应用主机访问控制端口。用户容器无网络、无 Docker socket、
无控制令牌和应用环境变量。

应用侧配置：

```sh
export HWOPS_RUNNER_URL=https://runner.internal:8091
export HWOPS_RUNNER_TOKEN_FILE=/受限目录/runner-token
export HWOPS_FILES_DIR=/持久化目录/workbench-files
export HWOPS_FILE_QUOTA_BYTES=10000000000
export HWOPS_FILE_RESERVE_BYTES=1000000000
```

runner 启动先运行真实隔离探测。gVisor、固定镜像、cgroup 控制器、禁网、只读输入、
临时空间或预装包任一不符时不监听；应用也拒绝启用执行，不回退到宿主 Python。
当前预装版本由 `deploy/runner/requirements.lock` 固定，不允许运行时 `pip install`。

## 独立附件 parser runner

附件解析使用第二个 runner 端点、令牌、任务目录和固定镜像，不与用户 Python
共享镜像或解释器。它仍强制实际 Linux + gVisor、无网络、只读输入和同一组硬资源
限制，但只运行应用内置解析程序，浏览器和模型不能提供解析代码。

```sh
sudo docker build -t hwops-parser:wb04 deploy/parser
sudo docker image inspect hwops-parser:wb04 --format '{{.Id}}'
sudo install -m 0644 deploy/parser/hwops-parser-runner.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now hwops-parser-runner
```

`/etc/hwops/parser.env` 设置固定镜像摘要与独立监听地址；
`/etc/hwops/parser-runner-token`、TLS 证书和私钥不得复用 Python runner 的文件。
应用侧设置：

```sh
export HWOPS_PARSER_RUNNER_URL=https://runner.internal:8092
export HWOPS_PARSER_RUNNER_TOKEN_FILE=/受限目录/parser-runner-token
```

parser 镜像额外固定 PyMuPDF 1.26.4。每次解析在全新容器内完成，结果作为有界 ZIP
回传；应用再次检查路径、条目数、展开字节数、媒体类型和 SHA256，再将预览及内嵌
图片保存为不可变对象。PDF 超过 200 页不截断；扫描页不做 OCR，并保留逐页缺口。

## 账号与访问规则

- 管理员在账号管理页创建账号、停用/启用和重置密码。角色为 ADMIN/USER，
  最后一个有效管理员不能被停用；管理员不能读取他人的私有聊天。
- 密码采用 bcrypt cost 12。登录会话保存凭据哈希，12 小时有效，每账号最多 20 个。
  Cookie 为 `__Host-hwops`、HttpOnly、Secure、SameSite=Strict、Path=/。
- 登录需匹配 Origin；其他修改请求另需 `X-CSRF-Token`，由登录响应或 `/v1/auth/me`
  返回。前端不使用 localStorage 保存密码或会话凭据。
- 登录限流保存在 PostgreSQL：每账号 15 分钟最多 5 次失败尝试；每网络对端
  15 分钟最多 30 次登录请求。成功后清除账号失败计数。网络对端取连接地址，
  不信任客户端传入的 X-Forwarded-For。
- 退出、停用、重置撤销相应凭据。SSE 每次读取前复核会话，撤销后关闭。
- 公共知识和设备写入限管理员；普通用户只读已发布资料及自己答案所引用的撤回资料。
  未建立用户归属的原诊断 HTTP 和诊断 worker 在团队模式关闭。
- 团队模式追踪默认仅保留标识、计数、模型与耗时；私有输入输出不导出。
  `HWOPS_TRACE_PRIVATE_CONTENT=true` 可为明确的非敏感合成联调启用内容追踪。

## 升级与历史归属

`migrations.Apply` 在事务中建立临时参考 schema，核对业务表实际列、约束和索引，
再接入旧 001–004 与新 005～008 迁移。`hwops_migrations` 保存连续版本和 SQL SHA256。
已有迁移内容或表结构不一致时启动失败，业务数据不被重建或覆盖。

升级前备份数据库；数据库账号需有建表、建 schema 和索引权限。没有自动删除数据的
向下迁移；需要回退时停止应用，恢复升级前备份并使用对应旧版本应用。

旧 `local-operator` 会话的 `owner_id` 保持 NULL，不进入任何新用户历史。管理员
明确选择目标账号后，离线执行：

```sh
go run ./cmd/hwops-admin -command map-legacy -username target-user
```

映射在一个事务中更新归属并记录 `ownership_mappings`。同一映射可重复运行，
不能把已映射历史悄悄转给另一个人。普通登录没有自动认领历史的行为。

## 备份、恢复与故障处理

[`deploy/app/backup.sh`](../deploy/app/backup.sh) 停止 `hwopsd` 后生成同一时间点的
PostgreSQL custom dump、私有文件 tar 和 SHA256 清单，再恢复服务。备份目的地必须在
`HWOPS_FILES_DIR` 之外，并由独立主机或对象存储按团队保留策略复制和加密：

```sh
sudo deploy/app/backup.sh /srv/backups/hwops
cd /srv/backups/hwops/hwops-时间戳
sha256sum --check SHA256SUMS
pg_restore --list database.dump >/dev/null
tar --list --gzip --file files.tar.gz >/dev/null
```

每次升级前执行并验证备份。恢复会删除目标数据库中当前 schema 对象并替换私有文件，
因此必须先隔离入口、确认目标环境和备份时间，再显式运行：

```sh
sudo deploy/app/restore.sh /srv/backups/hwops/hwops-时间戳 --confirm-restore
curl --fail --silent https://hwops.example.internal/healthz
```

恢复脚本先校验 hash，失败时保持应用停止；成功后旧文件目录保留为
`.files.pre-restore-*`，人工确认数据库、登录、附件和产物后再删除。升级回退使用旧
二进制、对应前端以及升级前的数据库/文件整组备份，不运行向下迁移。

常见故障处理：

- `migration history or checksum mismatch`：保持服务停止，核对发布包；不得修改已应用
  SQL 或跳过 checksum。
- 第二个 PostgreSQL 实例锁失败：停止多余实例；不得清除 advisory lock 后并行运行。
- runner capability、镜像或包不符：修复专用 runner 后再启动应用；不切换宿主执行。
- 文件额度或保留空间不足：停止新增上传，扩容或完成删除清理；不得降低 reserve
  继续写满磁盘。
- 模型限流或不可用：保留失败响应和执行记录，按上游限额恢复后由用户显式重试。

主存储删除不会擦除历史备份。备份到期和销毁必须按团队的数据保留策略独立执行并
留存审计记录。

## 会话、上下文与删除

浏览器支持新建、标题和正文搜索、改名、切换及删除；URL 的 `conversation` 参数
指向指定私有会话，刷新后从服务端恢复。未发输入在当前页面按会话保留。
发送冲突不会清空输入；断网后的同内容提交继续使用原幂等键。

消息按序追加，失败重试生成新响应并保存 `retry_of`。同会话只有一轮活跃响应，
其他会话可以并行。停止先进入 CANCELING，模型请求结束后成为 CANCELED。
服务重启将未完成记录标为 INTERRUPTED，用户显式重试才创建新尝试。

工作台每轮自受理起五分钟、最多二十次模型调用，包含历史摘要和模型 HTTP 重试。
最近六轮加较早历史摘要进入模型，保留原消息、覆盖范围和引用。历史设备读数
不作为新鲜数据使用。历史输入超限或摘要失败会明确报错，不静默遗漏历史。

部署时按实际模型能力设置：

| 配置 | 默认 | 约束 |
| --- | ---: | --- |
| `HWOPS_MODEL_CONTEXT_TOKENS` | 131072 | 不高于实际模型及网关允许容量 |
| `HWOPS_INPUT_TOKEN_BUDGET` | 98304 | 至少 4096，加 8192 输出预算后严格低于模型容量 |

输入采用 UTF-8 序列化字节数加工具及框架余量做保守 token 上界估算。
较长问题或较多引用可能提早触及预算；返回明确错误，原始历史保留。
摘要本体至多 6000 字节、含引用记录至多 24 KiB、完整历史选择至多 48 KiB。

删除立即禁止正常读取、搜索及 SSE，并停止活跃任务。`cleanup_jobs` 保存可重试
清理进度，当前清理消息、事件、摘要、附件、执行、runner 文件及产物，完成后保留最小
tombstone。解析中的附件先取消独立 runner 身份；原件和派生文件删除后才删除记录。
数据库备份按部署保留策略到期，主存储删除
不等于擦除已有备份。

## 验证

`HWOPS_TEST_DATABASE_URL` 指向专用测试 PostgreSQL（URL 形式）；测试账号需能
创建和删除临时数据库。WB 测试逐次创建独立数据库，结束后删除，不用业务库：

```sh
go test ./test/acceptance -run 'TestWB01|TestWB02|TestPostgreSQL' -v
go test -race ./...
go vet ./...
go build ./...
```

真实模型另加 `HWOPS_TEST_LIVE_MODEL=1`、`HWOPS_MODEL_ENDPOINT`、`HWOPS_MODEL`
及受控注入的 `HWOPS_MODEL_API_KEY`，运行 `TestWB01LiveModel|TestWB02LiveHistory`。它只验证合成资料的
模型协议与引用流程，不代表真实运维问答质量。

WB-03 的实际隔离测试还需专用 runner。下面的 URL 可由 SSH 隧道映射到 runner
回环监听；控制服务名称仅用于测试重启场景：

```sh
HWOPS_TEST_RUNNER_URL=http://127.0.0.1:8091 \
HWOPS_TEST_RUNNER_TOKEN_FILE=/受限目录/runner-token \
HWOPS_TEST_RUNNER_COLIMA_PROFILE=hwops-wb \
HWOPS_TEST_DATABASE_URL='postgres://...' \
go test ./test/acceptance -run '^TestWB03' -v -count=1
```

测试记录实际 Linux 内核、gVisor 版本、镜像摘要、探测哈希、执行代码/输入/产物哈希
及 cgroup 的 CPU、内存和进程峰值。未设置实际 runner 时测试明确跳过，不以协议桩
代替隔离验收。

WB-04 实际解析测试另需独立 parser runner：

```sh
HWOPS_TEST_PARSER_RUNNER_URL=http://127.0.0.1:8092 \
HWOPS_TEST_PARSER_RUNNER_TOKEN_FILE=/受限目录/parser-runner-token \
HWOPS_TEST_DATABASE_URL='postgres://...' \
go test ./test/acceptance -run '^TestWB04' -v -count=1
```

该组测试使用公开 HTTP、真实 PostgreSQL 与真实文件目录验证 PNG、日志、文字 PDF、
表格、内嵌图片、混合扫描页、损坏文件和 201 页拒绝。外部视觉模型桩仅标记 REPLAY；
真实视觉调用还需配置实际模型端点和凭据，并在报告中与解析器结果分开记录。

浏览器验收使用隔离的 Chrome 配置，不读取用户浏览器数据。先构建前端，再启动：

```sh
HWOPS_BROWSER_TEST=1 go test ./test/acceptance -run '^TestWB01BrowserFixture$' -v -count=1
```

控制台给出临时 localhost 地址及测试进程 PID。另一个终端在 `web/` 运行：

```sh
HWOPS_BROWSER_URL=http://localhost:给出的端口 npm run test:e2e
```

该测试实例使用明确的合成账号和外部 REPLAY 模型 Adapter。截图位于
`web/test-results/`，包括桌面、窄屏和原文抽屉。完成后向测试进程发送 SIGTERM，
它会关闭应用并删除临时库；最多存活 12 分钟。真实部署不可启用此测试实例。

WB-08 容量与恢复测试要求真实 PostgreSQL、实际 Python runner 以及 PostgreSQL 17
客户端。容量模型为外部 HTTP REPLAY，Python 为实际 gVisor；报告不得把两者混写：

```sh
HWOPS_TEST_DATABASE_URL='postgres://...' \
HWOPS_TEST_RUNNER_URL=http://127.0.0.1:8091 \
HWOPS_TEST_RUNNER_TOKEN_FILE=/受限目录/runner-token \
go test ./test/acceptance \
  -run '^(TestWB08TenActiveConversationsTwoPythonExecutions|TestWB08PostgresDumpAndPrivateFilesRestore)$' \
  -v -count=1
```

真实模型矩阵分别验证知识工具与多帧流式、自动 Python、实际图片字节。设置的模型
必须同时支持 OpenAI 兼容原生 tools、stream 和 image URL data URI：

```sh
HWOPS_TEST_LIVE_MODEL=1 \
HWOPS_MODEL_ENDPOINT=https://模型网关/v1/chat/completions \
HWOPS_MODEL=实际模型 \
HWOPS_MODEL_API_KEY="$(<受限密钥文件)" \
HWOPS_TEST_DATABASE_URL='postgres://...' \
HWOPS_TEST_RUNNER_URL=http://127.0.0.1:8091 \
HWOPS_TEST_RUNNER_TOKEN_FILE=/受限目录/runner-token \
HWOPS_TEST_PARSER_RUNNER_URL=http://127.0.0.1:8092 \
HWOPS_TEST_PARSER_RUNNER_TOKEN_FILE=/受限目录/parser-token \
go test ./test/acceptance \
  -run '^(TestWB01LiveModel|TestWB06LiveModelRunsActualPython|TestWB08LiveModelReadsPrivateImage)$' \
  -v -count=1
```

真实模型失败保持 LIVE 失败，不以 REPLAY 重试或填补。模型效果、现场设备状态、
PostgreSQL、parser 和 Python 隔离结果在验收报告中分别陈述。
