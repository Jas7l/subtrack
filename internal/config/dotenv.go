package config

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// DefaultEnvFile is read by LoadEnvFile when ENV_FILE is not set. The path is
// relative to the working directory the server is started from.
const DefaultEnvFile = ".env"

// LoadEnvFile copies KEY=VALUE pairs from an env file into the process
// environment so the per-layer Load functions see them. The file path comes
// from ENV_FILE, falling back to DefaultEnvFile.
//
// Variables already present in the environment win over the file, so a value
// set in the shell or the GoLand run configuration overrides .env. A missing
// file is not an error when ENV_FILE is unset (production sets real env vars);
// an explicitly named file that does not exist is.
//
// Returns the path that was loaded, or "" if no file was read.
//
// Supported syntax (a deliberately small subset of dotenv):
//
//	# comment line
//	KEY=value            # trailing comment after whitespace
//	export KEY=value
//	KEY="quoted # not a comment"   (single or double quotes, no escapes)
func LoadEnvFile() (string, error) {
	path, explicit := os.LookupEnv("ENV_FILE")
	if !explicit {
		path = DefaultEnvFile
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) && !explicit {
			return "", nil
		}
		return "", fmt.Errorf("config: env file: %w", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		key, val, ok, err := parseEnvLine(sc.Text())
		if err != nil {
			return "", fmt.Errorf("config: env file %s:%d: %w", path, n, err)
		}
		if !ok {
			continue
		}
		if _, set := os.LookupEnv(key); set {
			continue
		}
		if err := os.Setenv(key, val); err != nil {
			return "", fmt.Errorf("config: env file %s:%d: %w", path, n, err)
		}
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("config: env file %s: %w", path, err)
	}
	return path, nil
}

// parseEnvLine parses one line of an env file. ok is false for blank and
// comment lines.
func parseEnvLine(line string) (key, val string, ok bool, err error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false, nil
	}
	line = strings.TrimPrefix(line, "export ")
	key, val, found := strings.Cut(line, "=")
	key = strings.TrimSpace(key)
	if !found || key == "" || strings.ContainsAny(key, " \t") {
		return "", "", false, fmt.Errorf("expected KEY=VALUE, got %q", line)
	}
	val = strings.TrimSpace(val)

	if len(val) > 0 && (val[0] == '"' || val[0] == '\'') {
		end := strings.IndexByte(val[1:], val[0])
		if end < 0 {
			return "", "", false, fmt.Errorf("unterminated quote in %s", key)
		}
		return key, val[1 : end+1], true, nil
	}
	// An unquoted value ends at " #" so inline comments are dropped.
	if i := strings.Index(val, " #"); i >= 0 {
		val = strings.TrimSpace(val[:i])
	} else if i := strings.Index(val, "\t#"); i >= 0 {
		val = strings.TrimSpace(val[:i])
	}
	return key, val, true, nil
}
