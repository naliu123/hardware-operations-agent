package knowledge

import (
	"strings"
	"testing"

	"hwops/internal/domain"
)

func TestNewRevisionBoundsLargeSections(t *testing.T) {
	longLine := strings.Repeat("节点参数", maxFragmentBytes)
	revision, err := NewRevision(domain.RevisionInput{
		Title:         "大文档",
		Source:        "https://example.test/manual",
		Content:       "# 一级\n\n" + strings.Repeat("普通段落\n", 3000) + "\n## 二级\n" + longLine,
		Applicability: domain.Applicability{Scope: "GENERAL"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(revision.Fragments) < 3 {
		t.Fatalf("expected bounded fragments, got %d", len(revision.Fragments))
	}
	for _, fragment := range revision.Fragments {
		if len(fragment.Content) > maxFragmentBytes {
			t.Fatalf("fragment %q has %d bytes", fragment.Section, len(fragment.Content))
		}
		if fragment.StartLine < 1 || fragment.EndLine < fragment.StartLine {
			t.Fatalf("invalid line range: %+v", fragment)
		}
	}
}

func TestNewRevisionKeepsShortSectionsUnchanged(t *testing.T) {
	revision, err := NewRevision(domain.RevisionInput{
		Title:         "短文档",
		Source:        "https://example.test/manual",
		Content:       "# 标题\n第一段\n第二段\n## 操作\n执行检查。",
		Applicability: domain.Applicability{Scope: "GENERAL"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(revision.Fragments) != 2 {
		t.Fatalf("expected 2 fragments, got %d", len(revision.Fragments))
	}
	if revision.Fragments[0].Section != "标题" || revision.Fragments[0].Content != "第一段\n第二段" {
		t.Fatalf("unexpected first fragment: %+v", revision.Fragments[0])
	}
	if revision.Fragments[1].Section != "操作" || revision.Fragments[1].Content != "执行检查。" {
		t.Fatalf("unexpected second fragment: %+v", revision.Fragments[1])
	}
}

func TestNewRevisionAcceptsVerifiedSemanticFragments(t *testing.T) {
	content := "# 文档\n\n## 参数\n| 名称 | 说明 |\n| --- | --- |\n| timeout | 超时时间 |\n\n## 界面\n![](https://example.test/screen.png \"配置页\")\n单击保存。"
	revision, err := NewRevision(domain.RevisionInput{
		Title:         "语义文档",
		Source:        "https://example.test/manual",
		Content:       content,
		Applicability: domain.Applicability{Scope: "GENERAL"},
		Fragments: []domain.FragmentInput{
			{
				Section: "参数", StartLine: 4, EndLine: 6,
				Content: "| 名称 | 说明 |\n| --- | --- |\n| timeout | 超时时间 |",
				Representations: []domain.RetrievalRepresentation{
					{Kind: "QUESTION", Text: "timeout参数表示什么？"},
					{Kind: "QUESTION", Text: "如何设置超时时间？"},
					{Kind: "QUESTION", Text: "参数表中有哪些超时配置？"},
					{Kind: "TABLE", Text: "参数表说明timeout代表超时时间。"},
				},
			},
			{
				Section: "界面", StartLine: 9, EndLine: 10,
				Content: "![](https://example.test/screen.png \"配置页\")\n单击保存。",
				Representations: []domain.RetrievalRepresentation{
					{Kind: "QUESTION", Text: "配置完成后如何提交？"},
					{Kind: "QUESTION", Text: "界面操作的最后一步是什么？"},
					{Kind: "QUESTION", Text: "在哪里执行保存操作？"},
					{Kind: "IMAGE", Text: "配置页图片位于单击保存的操作步骤。"},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if revision.SchemaVersion != 2 || len(revision.Fragments) != 2 ||
		len(revision.Fragments[0].Representations) != 4 {
		t.Fatalf("semantic enrichment was not retained: %+v", revision)
	}
}

func TestNewRevisionRejectsUnverifiableSemanticFragments(t *testing.T) {
	base := domain.RevisionInput{
		Title: "语义文档", Source: "https://example.test/manual",
		Content:       "# 文档\n\n## 参数\n| 名称 | 说明 |\n| --- | --- |\n| timeout | 超时时间 |",
		Applicability: domain.Applicability{Scope: "GENERAL"},
		Fragments: []domain.FragmentInput{{
			Section: "参数", StartLine: 4, EndLine: 6,
			Content: "| 名称 | 说明 |\n| --- | --- |\n| timeout | 超时时间 |",
			Representations: []domain.RetrievalRepresentation{
				{Kind: "QUESTION", Text: "问题一？"},
				{Kind: "QUESTION", Text: "问题二？"},
				{Kind: "QUESTION", Text: "问题三？"},
			},
		}},
	}
	if _, err := NewRevision(base); err == nil || !strings.Contains(err.Error(), "table description") {
		t.Fatalf("table without description accepted: %v", err)
	}
	base.Fragments[0].Representations = append(base.Fragments[0].Representations,
		domain.RetrievalRepresentation{Kind: "TABLE", Text: "参数说明表。"})
	base.Fragments[0].Content = "模型改写过的内容"
	if _, err := NewRevision(base); err == nil || !strings.Contains(err.Error(), "original line range") {
		t.Fatalf("rewritten source fragment accepted: %v", err)
	}
}

func TestRetrievalContextKeepsSourceAssetsAndInsertsDerivedTextAtTheirPositions(t *testing.T) {
	fragment := domain.Fragment{
		StartLine: 10,
		EndLine:   15,
		Content: strings.Join([]string{
			"参数如下。",
			"| 参数 | 说明 |",
			"| --- | --- |",
			"| timeout | 超时时间 |",
			`![](https://example.test/screen.png "配置页")`,
			"单击保存。",
		}, "\n"),
		Representations: []domain.RetrievalRepresentation{
			{Kind: "QUESTION", Text: "timeout是什么？"},
			{Kind: "QUESTION", Text: "如何保存？"},
			{Kind: "TABLE_TEXT", Text: "参数timeout表示超时时间。", StartLine: 13, EndLine: 13},
			{Kind: "IMAGE", Text: "配置页显示保存按钮。", StartLine: 14, EndLine: 14},
		},
	}
	context := RetrievalContext(fragment)
	if !strings.Contains(context, "| timeout | 超时时间 |") ||
		!strings.Contains(context, `![](https://example.test/screen.png "配置页")`) ||
		!strings.Contains(context, "[表格说明] 参数timeout表示超时时间。") ||
		!strings.Contains(context, "[图片说明] 配置页显示保存按钮。") {
		t.Fatalf("asset context lost source or derived text: %q", context)
	}
	if strings.Index(context, "[表格说明]") > strings.Index(context, "![") {
		t.Fatalf("table description was not inserted at the source position: %q", context)
	}
}

func TestNewRevisionRejectsAssetRepresentationWithWrongSourcePosition(t *testing.T) {
	content := "# 文档\n\n## 界面\n普通文字。\n![](https://example.test/screen.png)"
	_, err := NewRevision(domain.RevisionInput{
		Title:         "视觉文档",
		Source:        "https://example.test/manual",
		Content:       content,
		Applicability: domain.Applicability{Scope: "GENERAL"},
		Fragments: []domain.FragmentInput{{
			Section: "界面", StartLine: 4, EndLine: 5,
			Content: "普通文字。\n![](https://example.test/screen.png)",
			Representations: []domain.RetrievalRepresentation{
				{Kind: "QUESTION", Text: "界面有什么？"},
				{Kind: "QUESTION", Text: "如何查看界面？"},
				{Kind: "IMAGE", Text: "界面显示配置项。", StartLine: 4, EndLine: 4},
			},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "image representation source range") {
		t.Fatalf("image description with a non-image source line accepted: %v", err)
	}
}
