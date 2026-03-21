package mcp

import (
	"context"
	"fmt"
)

// Scarf holds owner and session identity extracted from args or HTTP headers.
type Scarf struct {
	OwnerID   string
	SessionID string
}

// GetKey returns "ownerID:sessionID" format key
func (s Scarf) GetKey() string {
	return s.OwnerID + ":" + s.SessionID
}

type ctxScarfKey struct{}

// ContextWithScarf stores scarf into context.
func ContextWithScarf(ctx context.Context, sc Scarf) context.Context {
	return context.WithValue(ctx, ctxScarfKey{}, sc)
}

// ScarfFromContex retrieves scarf from context.
func ScarfFromContex(ctx context.Context) (Scarf, bool) {
	if s, ok := ctx.Value(ctxScarfKey{}).(Scarf); ok {
		return s, true
	}
	return Scarf{}, false
}

// ParseScarfFromArgs 从 args 中提取 owner_id 和 session_id，
// 并用 context 中已存在的 Scarf 值覆盖（如果 Scarf 非空）
func ParseScarfFromArgs(ctx context.Context, args map[string]any) (Scarf, error) {
	sc := Scarf{
		OwnerID:   getStringArg(args, "owner_id"),
		SessionID: getStringArg(args, "session_id"),
	}

	if existing, ok := ScarfFromContex(ctx); ok {
		if existing.OwnerID != "" {
			sc.OwnerID = existing.OwnerID
		}
		if existing.SessionID != "" {
			sc.SessionID = existing.SessionID
		}
	}

	if sc.OwnerID == "" || sc.SessionID == "" {
		return sc, fmt.Errorf("owner_id and session_id are required")
	}

	return sc, nil
}

func getStringArg(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}
