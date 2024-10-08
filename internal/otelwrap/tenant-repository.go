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

// TenantRepositoryWrapper wraps OpenTelemetry's span
type TenantRepositoryWrapper struct {
	profile.TenantRepository
	tracer trace.Tracer
	prefix string
}

// NewTenantRepositoryWrapper creates a wrapper
func NewTenantRepositoryWrapper(wrapped profile.TenantRepository, tracer trace.Tracer, prefix string) *TenantRepositoryWrapper {
	return &TenantRepositoryWrapper{
		TenantRepository: wrapped,
		tracer:           tracer,
		prefix:           prefix,
	}
}

// FetchTenant ...
func (w *TenantRepositoryWrapper) FetchTenant(ctx context.Context, id uuid.UUID) (a *profile.Tenant, err error) {
	ctx, span := w.tracer.Start(ctx, w.prefix+"FetchTenant")
	defer span.End()

	a, err = w.TenantRepository.FetchTenant(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return a, err
}
