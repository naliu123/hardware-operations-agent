package acceptance_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/application"
	"hwops/internal/blobstore"
	"hwops/internal/domain"
)

func TestWB06ImageAndPDFToolsUsePrivateSources(t *testing.T) {
	parser := actualAttachmentParser(t)
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var envelope struct {
			Stream bool `json:"stream"`
		}
		_ = json.Unmarshal(raw, &envelope)
		if !envelope.Stream {
			var input struct {
				Messages []struct {
					Content json.RawMessage `json:"content"`
				} `json:"messages"`
			}
			if json.Unmarshal(raw, &input) != nil || len(input.Messages) != 2 ||
				!bytes.Contains(input.Messages[1].Content, []byte(`"image_url"`)) ||
				!bytes.Contains(input.Messages[1].Content, []byte(`"data:image/png;base64,`)) {
				http.Error(w, "visual bytes missing", http.StatusBadRequest)
				return
			}
			wbReply(w, "图片显示一个合成蓝色状态灯；未读取任何外部链接。")
			return
		}
		var input struct {
			Messages []*schema.Message `json:"messages"`
		}
		if json.Unmarshal(raw, &input) != nil || len(input.Messages) < 2 {
			http.Error(w, "invalid stream request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		var payload chatmodel.ContextInput
		_ = json.Unmarshal([]byte(input.Messages[1].Content), &payload)
		var toolMessages []*schema.Message
		for _, message := range input.Messages {
			if message.Role == schema.Tool {
				toolMessages = append(toolMessages, message)
			}
		}
		if len(toolMessages) == 0 {
			if len(payload.Attachments) != 1 {
				http.Error(w, "attachment identity missing", http.StatusBadRequest)
				return
			}
			arguments := map[string]any{"attachment_id": payload.Attachments[0].ID, "page": 1}
			if strings.Contains(payload.Question, "图片") {
				arguments["prompt"] = "描述状态灯颜色与可见内容"
			}
			wb06ToolCall(w, "wb06-media-read", einofflowRead(), arguments)
			return
		}
		var result domain.AttachmentReadResult
		_ = json.Unmarshal([]byte(toolMessages[len(toolMessages)-1].Content), &result)
		if result.Status != "OK" || result.Source == nil {
			http.Error(w, "private media source missing", http.StatusBadRequest)
			return
		}
		text := "已读取文字 PDF 第一页并保留页码来源。"
		if strings.Contains(payload.Question, "图片") {
			if !strings.Contains(result.Visual, "蓝色状态灯") {
				http.Error(w, "visual result missing", http.StatusBadRequest)
				return
			}
			text = "图片中的合成状态灯为蓝色；视觉结果保留不确定性。"
		} else if result.Text == "" {
			http.Error(w, "PDF text missing", http.StatusBadRequest)
			return
		}
		wb06Final(w, domain.Draft{Claims: []domain.Claim{{
			Text: text, SourceIDs: []string{result.Source.SourceID},
		}}, Gaps: []string{}})
	}))
	defer modelServer.Close()
	cm, _ := chatmodel.NewOpenAI(modelServer.URL, "wb06-media-replay", "")
	files, err := blobstore.Open(filepath.Join(t.TempDir(), "private"), 500_000_000, 100_000_000)
	if err != nil {
		t.Fatal(err)
	}
	server, _, stop := wbStartOptions(t, workbenchDatabase(t), cm, true, application.Options{
		UsersMode: true, AttachmentRunner: parser, Files: files,
	})
	defer stop()
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)
	conversationID := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	png, _ := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	pdf := syntheticPDF([]string{"BT /F1 12 Tf 72 720 Td (Blue maintenance indicator) Tj ET"}, false)
	for _, item := range []struct {
		name, mediaType, question, expected string
		raw                                 []byte
	}{
		{"indicator.png", "image/png", "分析这张图片", "蓝色", png},
		{"manual.pdf", "application/pdf", "读取这个 PDF", "文字 PDF", pdf},
	} {
		uploaded := uploadAttachment(t, client, conversationID, item.name, item.mediaType, item.raw, 201)
		attachment := waitAttachment(t, client, uploaded["id"].(string))
		if attachment["status"] != "READY" {
			t.Fatalf("%s unavailable: %v", item.name, attachment)
		}
		submitted := client.request("POST", "/v1/conversations/"+conversationID+"/messages", map[string]any{
			"text": item.question, "attachment_ids": []string{uploaded["id"].(string)},
		}, 202)
		answer := client.answer(submitted["id"].(string), 60*time.Second)
		if answer["status"] != "ANSWERED" || !strings.Contains(answer["answer"].(string), item.expected) ||
			len(answer["sources"].([]any)) != 1 {
			t.Fatalf("%s analysis failed: %v", item.name, answer)
		}
	}
	t.Log("ACTUAL PARSER + REPLAY MODEL: private image bytes and PDF page text were read through bounded tools and cited by source_id")
}
