// Package config loads csync's per-project saved-target file, .csync.toml,
// which names the remote that `csync push` and `csync pull` sync against. The
// file lives in the project directory and is read from there; this package only
// parses it, leaving how to report a missing or malformed file to the caller.
package config

import (
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the parsed contents of a project's .csync.toml: the saved remote
// endpoint (user@host:/path) that push and pull transfer against.
type Config struct {
	Remote string `toml:"remote"`
}

// Load reads .csync.toml from dir and returns the saved target it defines. A
// missing or malformed file surfaces as an error for the caller to report;
// validating the resolved remote is the caller's responsibility.
func Load(dir string) (Config, error) {
	var c Config
	_, err := toml.DecodeFile(filepath.Join(dir, ".csync.toml"), &c)
	if err != nil {
		return Config{}, err
	}
	return c, nil
}
