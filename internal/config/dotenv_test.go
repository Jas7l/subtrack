package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseEnvLine(t *testing.T) {
	tests := []struct {
		line     string
		key, val string
		ok       bool
		wantErr  bool
	}{
		{line: "", ok: false},
		{line: "   # comment", ok: false},
		{line: "A=1", key: "A", val: "1", ok: true},
		{line: "export B = two ", key: "B", val: "two", ok: true},
		{line: "LOG_LEVEL=info  # debug | info", key: "LOG_LEVEL", val: "info", ok: true},
		{line: "EMPTY=", key: "EMPTY", val: "", ok: true},
		{line: `P="pa ss # x"  # comment`, key: "P", val: "pa ss # x", ok: true},
		{line: "Q='single'", key: "Q", val: "single", ok: true},
		{line: "URL=a#b", key: "URL", val: "a#b", ok: true},
		{line: "no equals sign", wantErr: true},
		{line: `X="open`, wantErr: true},
	}
	for _, tt := range tests {
		key, val, ok, err := parseEnvLine(tt.line)
		if (err != nil) != tt.wantErr {
			t.Errorf("%q: err = %v, wantErr %v", tt.line, err, tt.wantErr)
			continue
		}
		if key != tt.key || val != tt.val || ok != tt.ok {
			t.Errorf("%q: got (%q, %q, %v), want (%q, %q, %v)", tt.line, key, val, ok, tt.key, tt.val, tt.ok)
		}
	}
}

func TestLoadEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.env")
	content := "SUBTRACK_TEST_FROM_FILE=file\nSUBTRACK_TEST_PRESET=file\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENV_FILE", path)
	t.Setenv("SUBTRACK_TEST_PRESET", "shell")
	t.Setenv("SUBTRACK_TEST_FROM_FILE", "") // registers cleanup for the var
	os.Unsetenv("SUBTRACK_TEST_FROM_FILE")

	got, err := LoadEnvFile()
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Errorf("loaded %q, want %q", got, path)
	}
	if v := os.Getenv("SUBTRACK_TEST_FROM_FILE"); v != "file" {
		t.Errorf("SUBTRACK_TEST_FROM_FILE = %q, want %q", v, "file")
	}
	if v := os.Getenv("SUBTRACK_TEST_PRESET"); v != "shell" {
		t.Errorf("SUBTRACK_TEST_PRESET = %q, want shell value to win", v)
	}
}

func TestLoadEnvFileMissing(t *testing.T) {
	t.Setenv("ENV_FILE", filepath.Join(t.TempDir(), "nope.env"))
	if _, err := LoadEnvFile(); err == nil {
		t.Error("explicit missing ENV_FILE: want error")
	}
}
