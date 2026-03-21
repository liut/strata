package mcp

import (
	"context"
	"fmt"
)

type Scarf struct {
	UserID    string
	SessionID string
}

// GetKey returns "userID:sessionID" format key
func (s Scarf) GetKey() string {
	return s.UserID + ":" + s.SessionID
}

type ctxScarfKey struct{}

func ContextWithScarf(ctx context.Context, sc Scarf) context.Context {
	return context.WithValue(ctx, ctxScarfKey{}, sc)
}

func ScarfFromContex(ctx context.Context) (Scarf, bool) {
	if s, ok := ctx.Value(ctxScarfKey{}).(Scarf); ok {
		return s, true
	}
	return Scarf{}, false
}

// ParseScarfFromArgs 从 args 中提取 user_id 和 session_id，
// 并用 context 中已存在的 Scarf 值覆盖（如果 Scarf 非空）
func ParseScarfFromArgs(ctx context.Context, args map[string]any) (Scarf, error) {
	sc := Scarf{
		UserID:    getStringArg(args, "user_id"),
		SessionID: getStringArg(args, "session_id"),
	}

	if existing, ok := ScarfFromContex(ctx); ok {
		if existing.UserID != "" {
			sc.UserID = existing.UserID
		}
		if existing.SessionID != "" {
			sc.SessionID = existing.SessionID
		}
	}

	if sc.UserID == "" || sc.SessionID == "" {
		return sc, fmt.Errorf("user_id and session_id are required")
	}

	return sc, nil
}

func getStringArg(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}
