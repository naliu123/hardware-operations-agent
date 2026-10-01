# 工程验证记录

2026-09-20在工作区使用Go 1.26.8执行，race、vet和构建均通过。沙箱要求保留临时构建与测试目录，使用`-work`与`HWOPS_KEEP_TEST_ARTIFACTS=1`；这些目录位于忽略版本管理的`.cache/`。

| 验证 | 结果 | 日志 |
| --- | --- | --- |
| 全量`go test -work -race ./...`，含实际ES/embedding接入 | 通过，acceptance与contracts均成功 | [test-all.log](test-all.log) |
| 实际ES/embedding公开HTTP流程 | 通过 | [test-es.log](test-es.log) |
| 公开检索接口 | 通过 | [test-search.log](test-search.log) |
| DeepSeek模型HTTP协议与usage | 通过 | [test-model.log](test-model.log) |
| `go vet -work ./...` | 通过，无诊断 | [vet.log](vet.log) |
| Go应用构建 | 通过 | [build.log](build.log) |
| Phoenix/ES接口回读 | 2数据集、6实验、182 runs、607 spans；248 trace引用齐全 | [verification.json](../verification.json) |
| 资源清理 | 4服务及子进程退出，5端口释放，临时密钥删除 | [cleanup.json](../cleanup.json) |

集成测试使用真实临时文件存储；ES检索测试访问本地真实服务，生成器使用明确标记的REPLAY外部端点。DeepSeek真实生成、真实LLM评审与本表的确定性契约检查分别记录，见[主报表](../report.md)。

真实PostgreSQL没有配置连接，相关测试跳过；不以文件存储通过代替数据库验收。测试未下发任何设备命令。

后续复测先按[运行说明](../../../scripts/rag/README.md)启动依赖，再执行：

```sh
HWOPS_TEST_ES_URL=http://127.0.0.1:19200 \
HWOPS_TEST_EMBED_URL=http://127.0.0.1:18765 \
HWOPS_KEEP_TEST_ARTIFACTS=1 ./scripts/go.sh test -work -race ./... -count=1
./scripts/go.sh vet -work ./...
./scripts/go.sh build -work -o bin/ ./cmd/...
```

Python报表脚本已实际执行并检查图表；最终脚本语法、CSV行数、冻结数据哈希和文档链接检查结果见[artifact-checks.json](../artifact-checks.json)。报表重生成无需启动服务，也不会发起模型请求。
