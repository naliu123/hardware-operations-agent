package application

import (
	"context"
	"os"

	"hwops/internal/domain"
	"hwops/internal/sandbox"
)

// StartPython is the trusted application interface. The model-facing adapter
// supplies only code and file IDs; identity and response budget come from ctx
// and the durable response. It is intentionally not an HTTP execution endpoint.
func (a *App) StartPython(ctx context.Context, responseID, code string, inputIDs []string) (domain.PythonExecution, error) {
	if a.python == nil {
		return domain.PythonExecution{}, sandbox.ErrUnavailable
	}
	return a.python.Start(ctx, responseID, code, inputIDs)
}

func (a *App) WaitPython(ctx context.Context, id string) (domain.PythonExecution, error) {
	if a.python == nil {
		return domain.PythonExecution{}, sandbox.ErrUnavailable
	}
	return a.python.Wait(ctx, id)
}

func (a *App) PythonExecution(ctx context.Context, id string) (domain.PythonExecution, error) {
	if a.python == nil {
		return domain.PythonExecution{}, domain.ErrNotFound
	}
	return a.python.Get(ctx, id)
}

func (a *App) Artifact(ctx context.Context, id string) (domain.Artifact, *os.File, error) {
	if a.python == nil {
		return domain.Artifact{}, nil, domain.ErrNotFound
	}
	return a.python.Artifact(ctx, id)
}
