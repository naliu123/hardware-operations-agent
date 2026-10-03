// Package evaluation contains the portable, explicitly REPLAY QA baseline.
package evaluation

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"hwops/internal/domain"
)

type Baseline struct {
	SchemaVersion int               `json:"schema_version"`
	DataMode      string            `json:"data_mode"`
	CreatedAt     time.Time         `json:"created_at"`
	Model         string            `json:"model"`
	LiveStatus    string            `json:"live_status"`
	Revisions     []domain.Revision `json:"revisions"`
	Samples       []BaselineSample  `json:"samples"`
}

type BaselineSample struct {
	ID               string              `json:"id"`
	Input            domain.MessageInput `json:"input"`
	ExpectedStatus   string              `json:"expected_status"`
	ExpectedText     string              `json:"expected_text"`
	ForbiddenAnswer  string              `json:"forbidden_answer"`
	ExpectedDevice   string              `json:"expected_device,omitempty"`
	ExpectedRevision string              `json:"expected_revision,omitempty"`
	ElapsedMS        float64             `json:"elapsed_ms"`
	Metrics          map[string]bool     `json:"metrics"`
	Actual           domain.Response     `json:"actual"`
}

func (b Baseline) Markdown() (string, error) {
	if b.SchemaVersion != 1 || b.DataMode != "REPLAY" || len(b.Samples) == 0 || b.Model == "" {
		return "", fmt.Errorf("invalid or non-REPLAY baseline")
	}
	var out strings.Builder
	fmt.Fprintf(&out, "# QA-06 综合问答基线\n\n日期：%s；数据模式：**REPLAY**。\n\n模型配置：%s。真实模型效果：%s。\n\n",
		b.CreatedAt.Format(time.RFC3339), b.Model, b.LiveStatus)
	out.WriteString("公共HTTP入口、外部HTTP模型/监控桩与真实临时文件库；数值仅代表合成样本的确定性结构和规则验证。引用支持性检查为原文摘录与引用完整性，不是自然语言语义评审。生产设备、监控和PostgreSQL未联调。\n\n")
	out.WriteString("| 样本 | 预期 / 实际状态 | 检查 | 耗时 ms |\n| --- | --- | --- | ---: |\n")
	counts, passed := map[string]int{}, map[string]int{}
	var times []float64
	ids := map[string]bool{}
	failures := []string{}
	for _, sample := range b.Samples {
		if sample.ID == "" || ids[sample.ID] || sample.Actual.DataMode != "REPLAY" || len(sample.Metrics) == 0 || sample.ElapsedMS <= 0 {
			return "", fmt.Errorf("invalid sample %q", sample.ID)
		}
		ids[sample.ID] = true
		ok := true
		for metric, pass := range sample.Metrics {
			counts[metric]++
			if pass {
				passed[metric]++
			} else {
				ok = false
				failures = append(failures, sample.ID+"/"+metric)
			}
		}
		times = append(times, sample.ElapsedMS)
		fmt.Fprintf(&out, "| %s | %s / %s | %v | %.2f |\n",
			sample.ID, sample.ExpectedStatus, sample.Actual.Status, ok, sample.ElapsedMS)
	}
	out.WriteString("\n| 指标 | 通过 / 适用样本数 |\n| --- | ---: |\n")
	metrics := make([]string, 0, len(counts))
	for metric := range counts {
		metrics = append(metrics, metric)
	}
	slices.Sort(metrics)
	for _, metric := range metrics {
		fmt.Fprintf(&out, "| %s | %d / %d |\n", metric, passed[metric], counts[metric])
	}
	slices.Sort(times)
	fmt.Fprintf(&out, "\n端到端 p50 %.2f ms；p95 %.2f ms。失败检查：%v。\n\n",
		times[(len(times)-1)/2], times[(len(times)*95-1)/100], failures)
	out.WriteString("逐题输入、可接受结论、禁止回答、设备快照、知识修订及内容哈希、实际回答和指标保存在同目录 baseline.json。数据均为合成；该文件可供运维复核并作为诊断回归基准。\n")
	return out.String(), nil
}
