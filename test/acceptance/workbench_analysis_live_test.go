package acceptance_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/application"
	"hwops/internal/blobstore"
)

func TestWB06LiveModelRunsActualPython(t *testing.T) {
	if os.Getenv("HWOPS_TEST_LIVE_MODEL") != "1" {
		t.Skip("real model integration is separate; set HWOPS_TEST_LIVE_MODEL=1")
	}
	runner, parser := wbRunner(t), actualAttachmentParser(t)
	cm, err := chatmodel.NewOpenAI(os.Getenv("HWOPS_MODEL_ENDPOINT"), os.Getenv("HWOPS_MODEL"),
		os.Getenv("HWOPS_MODEL_API_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	files, err := blobstore.Open(filepath.Join(t.TempDir(), "private"), 500_000_000, 100_000_000)
	if err != nil {
		t.Fatal(err)
	}
	server, _, stop := wbStartOptions(t, workbenchDatabase(t), cm, true, application.Options{
		UsersMode: true, Runner: runner, AttachmentRunner: parser, Files: files,
	}, "LIVE")
	defer stop()
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)
	conversationID := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	raw := []byte("category,duration_ms\nstorage,120\nnetwork,80\nstorage,180\npower,40\n")
	uploaded := uploadAttachment(t, client, conversationID, "live-events.csv", "text/plain", raw, 201)
	attachment := waitAttachment(t, client, uploaded["id"].(string))
	if attachment["status"] != "READY" {
		t.Fatalf("LIVE attachment unavailable: %v", attachment)
	}
	submitted := client.request("POST", "/v1/conversations/"+conversationID+"/messages", map[string]any{
		"text": `只分析当前 CSV。必须调用 run_python_analysis 在隔离环境中按 category 统计数量和 duration_ms 总和，
将统计写入 /work/output/summary.csv，并生成 /work/output/duration.png 柱状图。
根据实际 stdout 回答，不能只给预期代码。`,
		"attachment_ids": []string{uploaded["id"].(string)},
	}, 202)
	answer := client.answer(submitted["id"].(string), 120*time.Second)
	if answer["status"] != "ANSWERED" && answer["status"] != "PARTIAL" {
		t.Fatalf("LIVE Python flow failed: status=%v error=%v", answer["status"], answer["error"])
	}
	executions, _ := answer["executions"].([]any)
	if len(executions) == 0 {
		t.Fatalf("LIVE model did not call Python: %v", answer)
	}
	last := executions[len(executions)-1].(map[string]any)
	result := last["result"].(map[string]any)
	if last["status"] != "SUCCEEDED" || !strings.Contains(result["stdout"].(string), "storage") ||
		len(result["artifacts"].([]any)) < 2 || len(answer["sources"].([]any)) == 0 {
		t.Fatalf("LIVE execution or provenance incomplete: %v", last)
	}
	t.Logf("LIVE: model=%s calls=%v execution=%s artifacts=%d answer_status=%s",
		os.Getenv("HWOPS_MODEL"), answer["model_usage"], last["id"],
		len(result["artifacts"].([]any)), answer["status"])
}

func TestWB08LiveModelReadsPrivateImage(t *testing.T) {
	if os.Getenv("HWOPS_TEST_LIVE_MODEL") != "1" {
		t.Skip("real visual model integration is separate; set HWOPS_TEST_LIVE_MODEL=1")
	}
	parser := actualAttachmentParser(t)
	cm, err := chatmodel.NewOpenAI(os.Getenv("HWOPS_MODEL_ENDPOINT"), os.Getenv("HWOPS_MODEL"),
		os.Getenv("HWOPS_MODEL_API_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	files, err := blobstore.Open(filepath.Join(t.TempDir(), "private"), 500_000_000, 100_000_000)
	if err != nil {
		t.Fatal(err)
	}
	server, _, stop := wbStartOptions(t, workbenchDatabase(t), cm, true, application.Options{
		UsersMode: true, AttachmentRunner: parser, Files: files,
	}, "LIVE")
	defer stop()
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)
	conversationID := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)

	value := image.NewRGBA(image.Rect(0, 0, 160, 100))
	for y := range 100 {
		for x := range 160 {
			value.Set(x, y, color.RGBA{R: 30, G: 105, B: 220, A: 255})
			if x >= 130 {
				value.Set(x, y, color.RGBA{R: 220, G: 45, B: 45, A: 255})
			}
		}
	}
	var encoded bytes.Buffer
	if err = png.Encode(&encoded, value); err != nil {
		t.Fatal(err)
	}
	uploaded := uploadAttachment(t, client, conversationID, "live-blue-red-panel.png", "image/png",
		encoded.Bytes(), 201)
	attachment := waitAttachment(t, client, uploaded["id"].(string))
	if attachment["status"] != "READY" {
		t.Fatalf("LIVE image unavailable: %v", attachment)
	}
	submitted := client.request("POST", "/v1/conversations/"+conversationID+"/messages", map[string]any{
		"text": `必须调用 read_private_attachment 读取当前图片字节。说明占面积最大的颜色，
并指出右侧窄条的颜色；不能根据文件名猜测。`,
		"attachment_ids": []string{uploaded["id"].(string)},
	}, 202)
	answer := client.answer(submitted["id"].(string), 120*time.Second)
	text := strings.ToLower(answer["answer"].(string))
	if (answer["status"] != "ANSWERED" && answer["status"] != "PARTIAL") ||
		(!strings.Contains(text, "蓝") && !strings.Contains(text, "blue")) ||
		(!strings.Contains(text, "红") && !strings.Contains(text, "red")) {
		t.Fatalf("LIVE visual answer failed: status=%v answer=%q error=%v",
			answer["status"], answer["answer"], answer["error"])
	}
	sources, _ := answer["sources"].([]any)
	if len(sources) == 0 {
		t.Fatalf("LIVE visual answer has no private source: %v", answer)
	}
	found := false
	for _, raw := range sources {
		source := raw.(map[string]any)
		if source["source_kind"] == "ATTACHMENT" && source["attachment_id"] == uploaded["id"] &&
			source["content_sha256"] != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("LIVE visual source does not bind the uploaded image: %v", sources)
	}
	t.Logf("LIVE VISION: model=%s attachment=%s bytes=%d usage=%v answer_status=%s",
		os.Getenv("HWOPS_MODEL"), uploaded["id"], encoded.Len(), answer["model_usage"], answer["status"])
}
