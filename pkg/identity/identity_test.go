package identity

import (
	"context"
	"net/http"
	"testing"
)

func TestScarf_GetKey(t *testing.T) {
	tests := []struct {
		name    string
		s       Scarf
		wantKey string
	}{
		{
			name:    "normal values",
			s:       Scarf{OwnerID: "user1", SessionID: "sess1"},
			wantKey: "user1:sess1",
		},
		{
			name:    "empty owner",
			s:       Scarf{OwnerID: "", SessionID: "sess1"},
			wantKey: ":sess1",
		},
		{
			name:    "empty session",
			s:       Scarf{OwnerID: "user1", SessionID: ""},
			wantKey: "user1:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.GetKey(); got != tt.wantKey {
				t.Errorf("Scarf.GetKey() = %v, want %v", got, tt.wantKey)
			}
		})
	}
}

func TestContextWithScarf(t *testing.T) {
	sc := Scarf{OwnerID: "user1", SessionID: "sess1"}
	ctx := ContextWithScarf(context.Background(), sc)

	got, ok := ScarfFromContext(ctx)
	if !ok {
		t.Error("ScarfFromContext() = false, want true")
	}
	if got != sc {
		t.Errorf("ScarfFromContext() = %v, want %v", got, sc)
	}
}

func TestScarfFromContext_NotFound(t *testing.T) {
	_, ok := ScarfFromContext(context.Background())
	if ok {
		t.Error("ScarfFromContext() = true, want false for empty context")
	}
}

func TestParseScarf(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		getters []func(key string) string
		wantSc  Scarf
		wantErr bool
	}{
		{
			name: "single getter - fromHeader",
			getters: []func(key string) string{
				FromHeader(http.Header{"X-Owner-Id": []string{"h-user"}, "X-Session-Id": []string{"h-sess"}}),
			},
			wantSc:  Scarf{OwnerID: "h-user", SessionID: "h-sess"},
			wantErr: false,
		},
		{
			name: "single getter - pathValue",
			getters: []func(key string) string{
				func(k string) string {
					if k == "uid" {
						return "p-user"
					}
					if k == "sid" {
						return "p-sess"
					}
					return ""
				},
			},
			wantSc:  Scarf{OwnerID: "p-user", SessionID: "p-sess"},
			wantErr: false,
		},
		{
			name: "header priority over path (later getter overrides)",
			getters: []func(key string) string{
				FromHeader(http.Header{"X-Owner-Id": []string{"h-user"}, "X-Session-Id": []string{"h-sess"}}),
				func(k string) string {
					if k == "uid" {
						return "p-user"
					}
					if k == "sid" {
						return "p-sess"
					}
					return ""
				},
			},
			wantSc:  Scarf{OwnerID: "p-user", SessionID: "p-sess"},
			wantErr: false,
		},
		{
			name: "context overrides all getters",
			ctx:  ContextWithScarf(context.Background(), Scarf{OwnerID: "ctx-user", SessionID: "ctx-sess"}),
			getters: []func(key string) string{
				FromHeader(http.Header{"X-Owner-Id": []string{"h-user"}, "X-Session-Id": []string{"h-sess"}}),
				func(k string) string {
					if k == "uid" {
						return "p-user"
					}
					if k == "sid" {
						return "p-sess"
					}
					return ""
				},
			},
			wantSc:  Scarf{OwnerID: "ctx-user", SessionID: "ctx-sess"},
			wantErr: false,
		},
		{
			name: "context partial override",
			ctx:  ContextWithScarf(context.Background(), Scarf{OwnerID: "ctx-user", SessionID: ""}),
			getters: []func(key string) string{
				FromHeader(http.Header{"X-Owner-Id": []string{""}, "X-Session-Id": []string{"h-sess"}}),
			},
			wantSc:  Scarf{OwnerID: "ctx-user", SessionID: "h-sess"},
			wantErr: false,
		},
		{
			name: "empty value does not override",
			getters: []func(key string) string{
				FromHeader(http.Header{"X-Owner-Id": []string{"h-user"}, "X-Session-Id": []string{""}}),
				func(k string) string {
					if k == "uid" {
						return ""
					}
					if k == "sid" {
						return "p-sess"
					}
					return ""
				},
			},
			wantSc:  Scarf{OwnerID: "h-user", SessionID: "p-sess"},
			wantErr: false,
		},
		{
			name: "missing owner_id",
			getters: []func(key string) string{
				FromHeader(http.Header{"X-Session-Id": []string{"h-sess"}}),
			},
			wantSc:  Scarf{SessionID: "h-sess"},
			wantErr: true,
		},
		{
			name: "missing session_id",
			getters: []func(key string) string{
				FromHeader(http.Header{"X-Owner-Id": []string{"h-user"}}),
			},
			wantSc:  Scarf{OwnerID: "h-user"},
			wantErr: true,
		},
		{
			name:    "all empty",
			getters: []func(key string) string{func(k string) string { return "" }},
			wantSc:  Scarf{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseScarf(tt.ctx, tt.getters...)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseScarf() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.wantSc {
				t.Errorf("ParseScarf() = %v, want %v", got, tt.wantSc)
			}
		})
	}
}
