package mcp

import (
	"context"

	"github.com/liut/strata/pkg/identity"
)

// Scarf is an alias for identity.Scarf for backwards compatibility.
type Scarf = identity.Scarf

// ContextWithScarf is an alias for identity.ContextWithScarf.
var ContextWithScarf = identity.ContextWithScarf

// ScarfFromContex is an alias for identity.ScarfFromContext.
// Deprecated: Use identity.ScarfFromContext instead.
var ScarfFromContex = identity.ScarfFromContext

// ParseScarfFromArgs 从 args 中提取 owner_id 和 session_id，
// 并用 context 中已存在的 Scarf 值覆盖（如果 Scarf 非空）
func ParseScarfFromArgs(ctx context.Context, args map[string]any) (Scarf, error) {
	return identity.ParseScarf(ctx, identity.FromArgs(args))
}
