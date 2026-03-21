package webapi

import (
	"bytes"
	"io"
	"testing"
	"time"
)

// mockSession 实现 SessionExecuter 接口
type mockSession struct {
	id    string
	input *bytes.Buffer
}

func (m *mockSession) ID() string            { return m.id }
func (m *mockSession) Read(b []byte) (int, error) {
	return m.input.Read(b)
}
func (m *mockSession) Write(b []byte) (int, error) {
	return len(b), nil
}

func TestExecInSession(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		cmd       string
		readData  string
		timeout   time.Duration
		wantOut   string
		wantTrunc bool
		wantErr   bool
	}{
		{
			name:      "simple output",
			sessionID: "sess123",
			cmd:       "echo hello",
			readData:  "echo hello\r\nhello\r\n__STRATA_EXEC_END_sess123__\r\n",
			timeout:   time.Second,
			wantOut:   "hello",
			wantTrunc: false,
			wantErr:   false,
		},
		{
			name:      "multiline output",
			sessionID: "sess456",
			cmd:       "ls -la",
			readData:  "ls -la\r\ntotal 8\r\ndrwxr-xr-x 2 user user 4096 Jan 20 10:00 .\r\n__STRATA_EXEC_END_sess456__\r\n",
			timeout:   time.Second,
			wantOut:   "total 8\r\ndrwxr-xr-x 2 user user 4096 Jan 20 10:00 .",
			wantTrunc: false,
			wantErr:   false,
		},
		{
			name:      "empty output",
			sessionID: "sess789",
			cmd:       "true",
			readData:  "true\r\n__STRATA_EXEC_END_sess789__\r\n",
			timeout:   time.Second,
			wantOut:   "",
			wantTrunc: false,
			wantErr:   false,
		},
		{
			name:      "timeout",
			sessionID: "sess timeout",
			cmd:       "sleep 10",
			readData:  "",
			timeout:   10 * time.Millisecond,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := &mockSession{
				id:    tt.sessionID,
				input: bytes.NewBufferString(tt.readData),
			}

			out, truncated, err := ExecInSession(sess, tt.cmd, tt.timeout)

			if (err != nil) != tt.wantErr {
				t.Errorf("ExecInSession() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if out != tt.wantOut {
					t.Errorf("ExecInSession() out = %q, want %q", out, tt.wantOut)
				}
				if truncated != tt.wantTrunc {
					t.Errorf("ExecInSession() truncated = %v, want %v", truncated, tt.wantTrunc)
				}
			}
		})
	}
}

func TestStripEcho(t *testing.T) {
	tests := []struct {
		name   string
		output []byte
		cmd    string
		want   string
	}{
		{
			name:   "single line with echo stripped",
			output: []byte("echo hello\r\nhello\r\n"),
			cmd:    "echo hello",
			want:   "hello",
		},
		{
			name:   "multiline keeps subsequent lines",
			output: []byte("echo hello\r\nhello\r\nworld\r\n"),
			cmd:    "echo hello",
			want:   "hello\r\nworld",
		},
		{
			name:   "no newline",
			output: []byte("hello"),
			cmd:    "echo hello",
			want:   "hello",
		},
		{
			name:   "trailing whitespace stripped",
			output: []byte("hello\r\nworld\r\n"),
			cmd:    "echo hello",
			want:   "world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripEcho(tt.output, tt.cmd)
			if string(got) != tt.want {
				t.Errorf("stripEcho() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExecInSessionWriteError(t *testing.T) {
	sess := &errorWriteSession{}
	_, _, err := ExecInSession(sess, "echo hello", time.Second)
	if err == nil {
		t.Error("ExecInSession() expected error for write failure, got nil")
	}
}

type errorWriteSession struct{}

func (e *errorWriteSession) ID() string    { return "err" }
func (e *errorWriteSession) Read(b []byte) (int, error) {
	return 0, io.EOF
}
func (e *errorWriteSession) Write(b []byte) (int, error) {
	return 0, io.ErrClosedPipe
}
