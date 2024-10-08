// Code generated by otelwrap; DO NOT EDIT.
// github.com/QuangTung97/otelwrap

package otelwrap

import (
	"context"
	"github.com/google/uuid"
	"github.com/telkomindonesia/go-boilerplate/internal/profile"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ProfileRepositoryWrapper wraps OpenTelemetry's span
type ProfileRepositoryWrapper struct {
	profile.ProfileRepository
	tracer trace.Tracer
	prefix string
}

// NewProfileRepositoryWrapper creates a wrapper
func NewProfileRepositoryWrapper(wrapped profile.ProfileRepository, tracer trace.Tracer, prefix string) *ProfileRepositoryWrapper {
	return &ProfileRepositoryWrapper{
		ProfileRepository: wrapped,
		tracer:            tracer,
		prefix:            prefix,
	}
}

// StoreProfile ...
func (w *ProfileRepositoryWrapper) StoreProfile(ctx context.Context, pr *profile.Profile) (err error) {
	ctx, span := w.tracer.Start(ctx, w.prefix+"StoreProfile")
	defer span.End()

	err = w.ProfileRepository.StoreProfile(ctx, pr)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

// FetchProfile ...
func (w *ProfileRepositoryWrapper) FetchProfile(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) (pr *profile.Profile, err error) {
	ctx, span := w.tracer.Start(ctx, w.prefix+"FetchProfile")
	defer span.End()

	pr, err = w.ProfileRepository.FetchProfile(ctx, tenantID, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return pr, err
}

// FindProfileNames ...
func (w *ProfileRepositoryWrapper) FindProfileNames(ctx context.Context, tenantID uuid.UUID, query string) (names []string, err error) {
	ctx, span := w.tracer.Start(ctx, w.prefix+"FindProfileNames")
	defer span.End()

	names, err = w.ProfileRepository.FindProfileNames(ctx, tenantID, query)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return names, err
}

// FindProfilesByName ...
func (w *ProfileRepositoryWrapper) FindProfilesByName(ctx context.Context, tenantID uuid.UUID, name string) (prs []*profile.Profile, err error) {
	ctx, span := w.tracer.Start(ctx, w.prefix+"FindProfilesByName")
	defer span.End()

	prs, err = w.ProfileRepository.FindProfilesByName(ctx, tenantID, name)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return prs, err
}
