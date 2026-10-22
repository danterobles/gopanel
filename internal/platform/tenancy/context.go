// Package tenancy carries only server-verified membership context.
package tenancy

import "context"

type ActorContext struct{ UserID, TenantID, TenantName, Role string }
type key struct{}

func WithActor(ctx context.Context, actor ActorContext) context.Context {
	return context.WithValue(ctx, key{}, actor)
}
func Actor(ctx context.Context) (ActorContext, bool) {
	actor, ok := ctx.Value(key{}).(ActorContext)
	return actor, ok
}
