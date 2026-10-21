package pg

import (
	"strings"
	"testing"
)

func TestValidate_ListsEveryMissingVar(t *testing.T) {
	err := Config{Host: "localhost"}.Validate()
	if err == nil {
		t.Fatal("expected an error for a config without credentials")
	}
	for _, want := range []string{"DB_USER", "DB_PASSWORD", "DB_NAME"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %s", err, want)
		}
	}
	if strings.Contains(err.Error(), "DB_HOST") {
		t.Errorf("error %q names DB_HOST, which is set", err)
	}
}

func TestDSN_EscapesPasswordAndPinsSchema(t *testing.T) {
	c := Config{Host: "db", Port: 5433, User: "u", Password: "p@ss/word", Database: "d", SSLMode: "disable"}
	dsn := c.DSN()
	if !strings.HasPrefix(dsn, "postgres://u:p%40ss%2Fword@db:5433/d?") {
		t.Errorf("unexpected DSN prefix: %s", dsn)
	}
	if !strings.Contains(dsn, "search_path=subtrack%2Cpublic") {
		t.Errorf("DSN does not pin search_path: %s", dsn)
	}
}

func TestLoad_RejectsMalformedPort(t *testing.T) {
	t.Setenv("DB_PORT", "not-a-number")
	if _, err := Load(); err == nil {
		t.Fatal("expected an error for a non-numeric DB_PORT")
	}
}
