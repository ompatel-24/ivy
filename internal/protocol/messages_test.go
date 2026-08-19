package protocol

import (
	"strings"
	"testing"
)

func TestDecodeResize(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    Resize
		wantErr bool
	}{
		{name: "valid", data: `{"type":"resize","cols":100,"rows":30}`, want: Resize{Type: "resize", Cols: 100, Rows: 30}},
		{name: "unknown type", data: `{"type":"input","cols":100,"rows":30}`, wantErr: true},
		{name: "unknown field", data: `{"type":"resize","cols":100,"rows":30,"extra":true}`, wantErr: true},
		{name: "missing dimension", data: `{"type":"resize","cols":100}`, wantErr: true},
		{name: "negative dimension", data: `{"type":"resize","cols":-1,"rows":30}`, wantErr: true},
		{name: "overflow dimension", data: `{"type":"resize","cols":65536,"rows":30}`, wantErr: true},
		{name: "multiple values", data: `{"type":"resize","cols":100,"rows":30} {}`, wantErr: true},
		{name: "invalid JSON", data: `{`, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := DecodeResize([]byte(test.data))
			if (err != nil) != test.wantErr {
				t.Fatalf("DecodeResize() error = %v, wantErr %v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("DecodeResize() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestDecodeResizeRejectsOversizeMessage(t *testing.T) {
	data := strings.Repeat(" ", MaxControlBytes+1)
	if _, err := DecodeResize([]byte(data)); err == nil {
		t.Fatal("DecodeResize() accepted an oversized message")
	}
}

func TestControlConstructors(t *testing.T) {
	metadata := SessionMetadata{ID: "id", Command: "bash", Directory: "/tmp", State: "running"}
	hello := NewHello(metadata)
	if hello.Type != "hello" || hello.Version != Version || hello.Session != metadata {
		t.Fatalf("NewHello() = %+v", hello)
	}
	if exit := NewExit(7); exit.Type != "exit" || exit.Code != 7 {
		t.Fatalf("NewExit() = %+v", exit)
	}
	if message := NewError("bad_message", "invalid"); message.Type != "error" || message.Code != "bad_message" {
		t.Fatalf("NewError() = %+v", message)
	}
}
