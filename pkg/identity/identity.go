package identity

import (
	"context"
	"fmt"
	"net/http"
)

const (
	// HeaderOwnerID is the HTTP header for owner identity.
	HeaderOwnerID = "X-Owner-Id"
	// HeaderSessionID is the HTTP header for session identity.
	HeaderSessionID = "X-Session-Id"

	keyOwnerID   = "uid"
	keySessionID = "sid"
)

// Scarf holds owner and session identity.
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

// ScarfFromContext retrieves scarf from context.
func ScarfFromContext(ctx context.Context) (Scarf, bool) {
	if ctx == nil {
		return Scarf{}, false
	}
	if s, ok := ctx.Value(ctxScarfKey{}).(Scarf); ok {
		return s, true
	}
	return Scarf{}, false
}

// FromHeader returns a getter function that reads from HTTP header.
func FromHeader(header http.Header) func(key string) string {
	return func(key string) string {
		switch key {
		case keyOwnerID:
			return header.Get(HeaderOwnerID)
		case keySessionID:
			return header.Get(HeaderSessionID)
		}
		return ""
	}
}

// FromArgs returns a getter function that reads from map args.
// Supports both "uid"/"sid" and "owner_id"/"session_id" keys.
func FromArgs(args map[string]any) func(key string) string {
	return func(key string) string {
		// Try direct key first
		if v, ok := args[key].(string); ok {
			return v
		}
		// Try alias keys
		switch key {
		case keyOwnerID:
			if v, ok := args["owner_id"].(string); ok {
				return v
			}
		case keySessionID:
			if v, ok := args["session_id"].(string); ok {
				return v
			}
		}
		return ""
	}
}

// ParseScarf extracts Scarf from getters.
// Later getters override earlier ones (non-empty values only).
// Context has the highest priority.
func ParseScarf(ctx context.Context, getters ...func(key string) string) (Scarf, error) {
	sc := Scarf{}

	// Getters: earlier has lower priority, later overrides
	for _, getter := range getters {
		if v := getter(keyOwnerID); v != "" {
			sc.OwnerID = v
		}
		if v := getter(keySessionID); v != "" {
			sc.SessionID = v
		}
	}

	// Context has highest priority
	if existing, ok := ScarfFromContext(ctx); ok {
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
