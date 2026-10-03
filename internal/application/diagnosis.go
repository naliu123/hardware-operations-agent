package application

import (
	"context"
	"hwops/internal/domain"
)

func (a *App) CreateIncident(ctx context.Context, in domain.IncidentInput, key string) (domain.Incident, error) {
	return a.diagnosis.CreateIncident(ctx, in, key)
}
func (a *App) CreateRun(ctx context.Context, incidentID string) (domain.DiagnosticRun, error) {
	return a.diagnosis.CreateRun(ctx, incidentID)
}
func (a *App) Run(ctx context.Context, id string) (domain.DiagnosticRun, error) {
	return a.diagnosis.Run(ctx, id)
}
func (a *App) ResumeRun(ctx context.Context, id string, in domain.RunResumeInput) (domain.DiagnosticRun, error) {
	return a.diagnosis.Resume(ctx, id, in)
}
func (a *App) CancelRun(ctx context.Context, id string, in domain.RunResumeInput) (domain.DiagnosticRun, error) {
	return a.diagnosis.Cancel(ctx, id, in)
}
