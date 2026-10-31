package service

import "context"

type explicitEgressEditKey struct{}

// WithExplicitEgressEdit is only used by authenticated administrative edits.
// Background refresh/import/fallback code must not acquire this authorization.
func WithExplicitEgressEdit(ctx context.Context, ids ...int64) context.Context {
	allowed := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id > 0 {
			allowed[id] = true
		}
	}
	return context.WithValue(ctx, explicitEgressEditKey{}, allowed)
}
func IsExplicitEgressEdit(ctx context.Context, id int64) bool {
	if ctx == nil {
		return false
	}
	allowed, _ := ctx.Value(explicitEgressEditKey{}).(map[int64]bool)
	return allowed[id]
}

// A separate capability authorizes resolving this administrator's filter once.
// It never authorizes any repository write until narrowed to concrete IDs.
type explicitFilteredEgressEditKey struct{}

func WithExplicitFilteredEgressEdit(ctx context.Context) context.Context {
	return context.WithValue(ctx, explicitFilteredEgressEditKey{}, true)
}

func resolveExplicitFilteredEgressEdit(ctx context.Context, ids []int64) context.Context {
	allowed, _ := ctx.Value(explicitFilteredEgressEditKey{}).(bool)
	if !allowed {
		return ctx
	}
	ctx = context.WithValue(ctx, explicitFilteredEgressEditKey{}, false)
	return WithExplicitEgressEdit(ctx, ids...)
}
