package model

import "context"

type TaskProgress struct {
	Phase   string `json:"phase"`
	Message string `json:"message,omitempty"`
	Current int    `json:"current,omitempty"`
	Total   int    `json:"total,omitempty"`
	Unit    string `json:"unit,omitempty"`
}

type TaskProgressReporter interface {
	Report(ctx context.Context, progress TaskProgress) error
}

type taskProgressReporterKey struct{}

func WithTaskProgressReporter(
	ctx context.Context,
	reporter TaskProgressReporter,
) context.Context {
	if reporter == nil {
		return ctx
	}

	return context.WithValue(ctx, taskProgressReporterKey{}, reporter)
}

func ReportTaskProgress(ctx context.Context, progress TaskProgress) error {
	if ctx == nil {
		return nil
	}

	reporter, ok := ctx.Value(taskProgressReporterKey{}).(TaskProgressReporter)
	if !ok || reporter == nil {
		return nil
	}

	return reporter.Report(ctx, progress)
}
