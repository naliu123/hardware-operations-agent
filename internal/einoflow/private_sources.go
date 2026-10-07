package einoflow

import (
	"context"

	"hwops/internal/domain"
)

func validateSourceIDs(ctx context.Context, store domain.Repository, t *turn, ids []string) error {
	allowed := make(map[string]domain.SourceRef, len(t.Response.Sources))
	for _, source := range t.Response.Sources {
		if source.SourceID == "" || allowed[source.SourceID].SourceID != "" {
			return ErrInvalidAnswer
		}
		allowed[source.SourceID] = source
	}
	seen := map[string]bool{}
	for _, id := range ids {
		source, ok := allowed[id]
		if !ok || seen[id] {
			return ErrInvalidAnswer
		}
		seen[id] = true
		switch source.SourceKind {
		case "ATTACHMENT":
			repo, ok := store.(domain.AttachmentRepository)
			if !ok {
				return ErrInvalidAnswer
			}
			attachment, err := repo.GetAttachment(ctx, source.AttachmentID)
			if err != nil {
				return err
			}
			if attachment.ConversationID != t.Response.ConversationID ||
				attachment.SHA256 != source.AttachmentHash ||
				(attachment.Status != "READY" && attachment.Status != "PARTIAL") {
				return ErrInvalidAnswer
			}
		case "EXECUTION":
			repo, ok := store.(domain.PythonRepository)
			if !ok {
				return ErrInvalidAnswer
			}
			execution, err := repo.GetPythonExecution(ctx, source.ExecutionID)
			if err != nil {
				return err
			}
			expected := domain.ExecutionSource(execution)
			if execution.ResponseID != t.Response.ID || execution.ConversationID != t.Response.ConversationID ||
				execution.Status != "SUCCEEDED" || !execution.Result.Cleaned ||
				expected.SourceID != source.SourceID || expected.ContentSHA256 != source.ContentSHA256 ||
				expected.CodeSHA256 != source.CodeSHA256 {
				return ErrInvalidAnswer
			}
		default:
			return ErrInvalidAnswer
		}
	}
	return nil
}
