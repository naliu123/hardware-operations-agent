"""Freeze source-grounded Chinese questions before running retrieval experiments."""

import hashlib
import json
from pathlib import Path


QUESTIONS = {
    "cce-t1-r01": "CCE 管理面内存突然升高，大量客户端在全量拉取资源列表，该怎么处理？",
    "cce-t1-r02": "有人改了名字带 cce-control 的安全组，主控节点不可用了，应参照什么恢复？",
    "cce-t1-r03": "控制节点过期销毁了，文档认为能恢复吗？",
    "cce-t1-r04": "给控制节点重新装了一遍系统后组件不见了，可以用重置节点恢复吗？",
    "cce-t1-r05": "自行把控制节点的 etcd 升了版本，集群出问题后怎么办？",
    "cce-t1-r06": "误格式化控制节点的 /etc/kubernetes 数据，文档给出的恢复结论是什么？",
    "cce-t1-r07": "CCE 控制节点换 IP 后不可用，应该怎么恢复？",
    "cce-t1-r08": "手工调过 kube-apiserver 和 etcd 的启动参数后控制节点异常，应该恢复到什么配置？",
    "cce-t1-r09": "自行给控制节点换了一套 etcd 证书导致集群不可用，能恢复吗？",
    "cce-t1-r10": "工作节点 cce-node 安全组被调整后无法使用，应该如何修复？",
    "cce-t1-r11": "想让业务用自建域名服务器，直接改宿主机 resolv.conf 有什么风险？正确做法是什么？",
    "cce-t1-r12": "工作节点被误删了，文档对恢复是怎么说的？",
    "cce-t1-r13": "节点绑定的弹性网卡删掉了会影响什么，还能恢复吗？",
    "cce-t1-r14": "普通 Node 节点被重新安装操作系统后组件丢失，应采取什么恢复动作？",
    "cce-t1-r15": "可以直接在 CCE 工作节点执行 yum update 更新内核和运行时吗？出问题怎么办？",
    "cce-t1-r16": "修改工作节点的地址后节点变为不可用，文档的解决办法是什么？",
    "cce-t1-r17": "直接改 kubelet 和 kube-proxy 参数以后，当时正常，升级集群时还会有什么风险？",
    "cce-t1-r18": "修改了节点操作系统配置后节点不可用，可以尝试哪两种处理方式？",
    "cce-t1-r19": "清理 /opt/cloud/cce 和 /var/paas 时误删了内容，节点不可用了怎么恢复？",
    "cce-t1-r20": "能自行更改 /etc/sudoers.d/sudoerspaas 和容器运行目录的权限吗？误改后怎么办？",
    "cce-t1-r21": "给 kubelet 使用的数据盘重新分区可能造成什么后果，应如何恢复？",
    "cce-t1-r22": "在工作节点安装额外的软件以后调度不了工作负载，应该先做什么？",
    "cce-t1-r23": "改动 NetworkManager 的配置后节点失联，文档建议怎么处理？",
    "cce-t1-r24": "清理了 cce-pause 系统镜像后无法创建容器，而且拉不回来镜像，怎么办？",
    "cce-t1-r25": "绕过节点池，在 ECS 控制台改节点规格，为什么弹性扩缩容数量可能不符合预期？",
    "cce-t1-r26": "误删 /usr/local/bin/crictl 后 containerd 一直重启，是什么原因，如何修复？",
    "cce-t1-r27": "自己创建或改动 paas 用户组，会影响哪些对象，应怎样恢复？",
    "cce-t2-r01": "把 IPv4 转发开关设成 0 后网络不通，应改成什么值？",
    "cce-t2-r02": "启用 tcp_tw_recycle 导致 NAT 出现异常，应该设回多少？",
    "cce-t2-r03": "tcp_tw_reuse 设置为 1 后出现网络异常，这篇文档建议如何处理？",
    "cce-t2-r04": "集群里 DNS 解析不正常，安全组应检查容器网段的哪个端口和协议？",
    "cce-t2-r05": "误删 default-network 的 network-attachment-definitions CRD，会影响什么，如何恢复？",
    "cce-t2-r06": "节点打开 iptables 防火墙后容器之间不通，需要关闭什么并检查哪些规则文件？",
    "cce-t2-r07": "隧道网络集群里能把 openvswitch 换成发行版自带的新版吗？",
    "cce-t3-r01": "特权容器运行 systemctl 后宿主机挂载点异常，可能是什么操作造成的，怎么恢复？",
    "cce-t3-r02": "用 hostPath 挂载 /var/lib/docker，为什么系统组件重启后容器可能读不了原内容？",
    "cce-t4-r01": "在 ELB 控制台直接删除已绑定 CCE 的负载均衡实例会有什么影响？",
    "cce-t4-r02": "暂时在 ELB 控制台停用已绑定 CCE 的实例，会影响 Service 和 Ingress 吗？",
    "cce-t4-r03": "从 ELB 控制台改了负载均衡器的私有 IPv4 地址，会影响流量和 YAML 哪些内容？",
    "cce-t4-r04": "把 CCE 使用的负载均衡器公网 IP 解绑后，它会变成什么类型，公网流量还能转发吗？",
    "cce-t4-r05": "在 CCE 自动创建的 ELB 上手动加了监听器，删除 Service 时为什么 ELB 可能删不掉？",
    "cce-t4-r06": "有人在 ELB 控制台删除了 CCE 自动创建的监听器，业务访问不通应如何修复？",
    "cce-t4-r07": "在 ELB 侧改了 CCE 监听器的名称和超时时间，集群升级后能保证保留吗？",
    "cce-t4-r08": "手工从监听器后端组删除一台服务器又添加一台，CCE 控制节点重启后这些更改会怎样？",
    "cce-t4-r09": "直接替换 CCE 监听器关联的整个后端服务器组，可能有什么后果，应该怎么处理？",
    "cce-t4-r10": "在 ELB 侧手工改了 Ingress 产生的转发规则，控制节点重启时会发生什么？",
    "cce-t4-r11": "要更换 CCE 监听器的服务器证书，怎样更新才能避免集群升级时被覆盖？",
    "cce-t5-r01": "删掉日志采集器的 pos 目录后日志为什么可能重复收集？",
    "cce-t5-r02": "清理宿主机上的 ccs-log-collector/buffer 目录会丢失什么？",
    "cce-t6-r01": "100 个节点的集群，云原生监控采集分片通常该配多少，配太多有什么风险？",
    "cce-t7-r01": "在控制台把正在用的 EVS 卸载后 Pod 写入报 IO Error，文档建议怎样处理？",
    "cce-t7-r02": "在节点直接 umount 云盘挂载路径后，Pod 的数据可能写到哪儿，该怎么恢复？",
    "cce-t7-r03": "绕过容器平台在节点上直接操作 EVS，会让 Pod 写到哪里，文档是否提供恢复方法？",
    "cce-t7-r04": "创建 PV 的 YAML 带着 status、spec.claimRef 和 set-disk-metadata，会有什么风险，删除前如何纠正？",
    "cce-t8-r01": "直接在后台改插件资源后升级丢了配置，应该通过哪些入口修改插件？",
}

MULTI = [
    ("重装控制节点和普通工作节点的操作系统，文档给出的恢复结论有什么不同？",
     ["cce-t1-r04", "cce-t1-r14"]),
    ("修改控制节点 IP 和修改 Node 节点 IP 后，文档各建议怎样恢复？",
     ["cce-t1-r07", "cce-t1-r16"]),
    ("集群控制节点与工作节点的安全组分别按什么规则命名，误改后如何修复？",
     ["cce-t1-r02", "cce-t1-r10"]),
    ("日志采集器的 pos 和 buffer 目录误删后，造成的结果分别是什么？",
     ["cce-t5-r01", "cce-t5-r02"]),
    ("在 ELB 侧删除整个 CCE 负载均衡实例与删除 CCE 自动创建的监听器，文档给出的建议有何区别？",
     ["cce-t4-r01", "cce-t4-r06"]),
    ("tcp_tw_recycle 和 tcp_tw_reuse 设成 1 后分别可能产生什么问题，文档建议的值是什么？",
     ["cce-t2-r02", "cce-t2-r03"]),
]

UNANSWERABLE = [
    "根据这篇文档，CCE 集群按小时收费的准确价格是多少？",
    "这篇文档明确要求升级到哪个 Kubernetes 小版本才能修复 CVE-2026-12345？",
    "帮我判断生产集群现在的 CPU 使用率是否正常，并给出实时数值。",
    "本文列出了所有地区节点操作系统镜像的 SHA256 校验值吗？请给出这些值。",
    "我的集群现在有多少个故障 Pod？请直接报告实际数量。",
]


def main():
    folder = Path("testdata/rag/cce")
    target = folder / "dataset.json"
    if target.exists():
        raise SystemExit("Dataset already frozen; create an explicitly versioned dataset to change it.")
    evidence = {e["evidence_id"]: e for e in json.loads((folder / "evidence.json").read_text())}
    if set(QUESTIONS) != set(evidence):
        raise ValueError("Every source row must have one independently phrased question.")
    groups = {key: key for key in evidence}
    for _, ids in MULTI:
        group = min(groups[key] for key in ids)
        old = {groups[key] for key in ids}
        for key in groups:
            if groups[key] in old:
                groups[key] = group
    ranked_groups = sorted(set(groups.values()), key=lambda x: hashlib.sha256(("cce-v1:"+x).encode()).hexdigest())
    development = set(ranked_groups[:len(ranked_groups)*2//5])
    examples = []
    for i, (question, ids) in enumerate(
        [(question, [key]) for key, question in QUESTIONS.items()] + MULTI, 1
    ):
        reference = "\n\n".join(
            f"[{key}] {evidence[key]['category']}\n{evidence[key]['text']}" for key in ids
        )
        examples.append({
            "id": f"cce-q{i:03}",
            "question": question,
            "reference_answer": reference,
            "relevant_evidence_ids": ids,
            "answerable": True,
            "split": "development" if groups[ids[0]] in development else "heldout",
            "section": evidence[ids[0]]["section"],
            "question_type": "comparison" if len(ids)>1 else "single",
        })
    for i, question in enumerate(UNANSWERABLE):
        examples.append({
            "id": f"cce-u{i+1:03}",
            "question": question,
            "reference_answer": "该页面没有足够依据回答此问题，应说明缺口，不能编造。",
            "relevant_evidence_ids": [],
            "answerable": False,
            "split": "development" if i<2 else "heldout",
            "section": "文档范围之外",
            "question_type": "unanswerable",
        })
    raw = json.dumps(examples, ensure_ascii=False, indent=2) + "\n"
    target.write_text(raw)
    manifest = {
        "version": "cce-v1",
        "sha256": hashlib.sha256(raw.encode()).hexdigest(),
        "total": len(examples),
        "development": sum(e["split"] == "development" for e in examples),
        "heldout": sum(e["split"] == "heldout" for e in examples),
        "answerable": sum(e["answerable"] for e in examples),
        "primary_metric": "macro Recall@5 on answerable heldout questions",
        "target": 0.60,
        "provenance": "Agent-authored questions grounded in captured official source rows; not production traffic or human-expert-certified labels.",
        "split_policy": "Deterministic source-evidence groups; every multi-evidence comparison stays in its source group's split.",
        "optimization_policy": "Inspect development bad cases only; freeze retrieval configuration before heldout evaluation.",
    }
    (folder / "dataset-manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2)+"\n")
    print(json.dumps(manifest, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
