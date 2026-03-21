package mcp

import (
	"context"
	"testing"
)

func TestParseScarfFromArgs(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		args    map[string]any
		wantUID string
		wantSID string
		wantErr bool
	}{
		{
			name:    "args provided",
			ctx:     context.Background(),
			args:    map[string]any{"user_id": "u1", "session_id": "s1"},
			wantUID: "u1",
			wantSID: "s1",
			wantErr: false,
		},
		{
			name:    "args empty strings",
			ctx:     context.Background(),
			args:    map[string]any{"user_id": "", "session_id": ""},
			wantErr: true,
		},
		{
			name:    "args missing",
			ctx:     context.Background(),
			args:    map[string]any{},
			wantErr: true,
		},
		{
			name: "context overrides empty args",
			ctx: ContextWithScarf(context.Background(), Scarf{UserID: "ctx_u1", SessionID: "ctx_s1"}),
			args: map[string]any{"user_id": "", "session_id": ""},
			wantUID: "ctx_u1",
			wantSID: "ctx_s1",
			wantErr: false,
		},
		{
			name:    "context overrides partial args",
			ctx:     ContextWithScarf(context.Background(), Scarf{UserID: "ctx_u1", SessionID: ""}),
			args:    map[string]any{"user_id": "", "session_id": "args_s1"},
			wantUID: "ctx_u1",
			wantSID: "args_s1",
			wantErr: false,
		},
		{
			name: "context takes precedence over args",
			ctx:  ContextWithScarf(context.Background(), Scarf{UserID: "ctx_u1", SessionID: "ctx_s1"}),
			args: map[string]any{"user_id": "args_u1", "session_id": "args_s1"},
			wantUID: "ctx_u1",
			wantSID: "ctx_s1",
			wantErr: false,
		},
		{
			name:    "non-string types ignored",
			ctx:     context.Background(),
			args:    map[string]any{"user_id": 123, "session_id": nil},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseScarfFromArgs(tt.ctx, tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseScarfFromArgs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got.UserID != tt.wantUID {
					t.Errorf("ParseScarfFromArgs() UserID = %v, want %v", got.UserID, tt.wantUID)
				}
				if got.SessionID != tt.wantSID {
					t.Errorf("ParseScarfFromArgs() SessionID = %v, want %v", got.SessionID, tt.wantSID)
				}
			}
		})
	}
}

func TestScarfFromContex(t *testing.T) {
	t.Run("with scarf", func(t *testing.T) {
		ctx := ContextWithScarf(context.Background(), Scarf{UserID: "u1", SessionID: "s1"})
		sc, ok := ScarfFromContex(ctx)
		if !ok {
			t.Error("ScarfFromContex() ok = false, want true")
		}
		if sc.UserID != "u1" || sc.SessionID != "s1" {
			t.Errorf("ScarfFromContex() = %v, want {u1, s1}", sc)
		}
	})

	t.Run("without scarf", func(t *testing.T) {
		ctx := context.Background()
		_, ok := ScarfFromContex(ctx)
		if ok {
			t.Error("ScarfFromContex() ok = true, want false")
		}
	})
}

func TestContextWithScarf(t *testing.T) {
	ctx := ContextWithScarf(context.Background(), Scarf{UserID: "u1", SessionID: "s1"})
	sc, ok := ScarfFromContex(ctx)
	if !ok {
		t.Fatal("ScarfFromContex() returned false")
	}
	if sc != (Scarf{UserID: "u1", SessionID: "s1"}) {
		t.Errorf("ScarfFromContex() = %v, want {u1, s1}", sc)
	}
}

func TestGetStringArg(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		key  string
		want string
	}{
		{"string value", map[string]any{"k": "v"}, "k", "v"},
		{"non-string", map[string]any{"k": 123}, "k", ""},
		{"nil", map[string]any{"k": nil}, "k", ""},
		{"missing key", map[string]any{}, "k", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getStringArg(tt.args, tt.key); got != tt.want {
				t.Errorf("getStringArg() = %v, want %v", got, tt.want)
			}
		})
	}
}
