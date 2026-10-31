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
