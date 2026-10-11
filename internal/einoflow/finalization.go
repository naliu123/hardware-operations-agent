package einoflow

import (
	"encoding/json"

	"github.com/cloudwego/eino/schema"
)

// A tool limit ends evidence gathering, not answer validation. The next model
// call is forbidden from using tools and still goes through the normal validator.
func beginFinalization(t *turn) {
	if t.Finalizing {
		return
	}
	t.Finalizing = true
	t.Messages = append(t.Messages, schema.UserMessage(`本轮证据收集已结束，不再调用任何工具。
仅根据本轮工具实际返回的证据回答原问题，继续使用规定的 JSON、claims 与真实来源 ID；
不得引用未执行的调用，不得把草稿或历史回答当作证据，不使用 reply 绕过事实校验。
已有证据足以回答时正常回答；仅能回答部分时保留有依据部分并列出具体缺口；
没有足够依据时输出 claims=[] 与具体 gaps，直接说明需要补充的资料、型号或输入。
不要在正文展示工具预算、调用限制、内部错误或检索循环。`))
}

// A rejected batch must still satisfy the upstream tool-message protocol.
// Successful calls earlier in the same batch already have a result.
func completeRejectedCalls(t *turn) {
	answered := map[string]bool{}
	for _, message := range t.Messages {
		if message.Role == schema.Tool {
			answered[message.ToolCallID] = true
		}
	}
	raw, _ := json.Marshal(map[string]any{
		"status": "NOT_EXECUTED",
		"error": map[string]string{
			"code":    "TOOL_BUDGET_EXCEEDED",
			"message": "本轮已停止收集证据，未执行该调用。仅使用已经返回的证据收尾。",
		},
	})
	for _, call := range t.Last.ToolCalls {
		if !answered[call.ID] {
			t.Messages = append(t.Messages, schema.ToolMessage(string(raw), call.ID))
		}
	}
}
