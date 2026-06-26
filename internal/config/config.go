// Package config loads csync's per-project saved-target file, .csync.toml,
// which names the remote that `csync push` and `csync pull` sync against. The
// file lives in the project directory and is read from there. Load is the single
// gate: it refuses a missing, malformed, or remote-less file rather than letting
// a default or empty operand reach rsync.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the parsed contents of a project's .csync.toml: the saved remote
// endpoint (user@host:/path) that push and pull transfer against.
type Config struct {
	Remote string `toml:"remote"`
}

// Load reads .csync.toml from dir and returns the saved target it defines. It
// rejects the three ways a config can be unusable, each with a distinct message:
// the file is missing, its TOML is malformed, or it sets no remote. The last
// guards SECURITY.md's empty-operand invariant — an empty remote becomes "/"
// once rsync appends a trailing slash — so it is rejected here, never defaulted,
// mirroring cli.Parse's empty-path check for command-line operands.
func Load(dir string) (Config, error) {
	var c Config
	_, err := toml.DecodeFile(filepath.Join(dir, ".csync.toml"), &c)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, fmt.Errorf("no .csync.toml found in %s", dir)
		}
		return Config{}, fmt.Errorf("invalid .csync.toml: %w", err)
	}
	if c.Remote == "" {
		return Config{}, fmt.Errorf(".csync.toml has no remote set")
	}
	return c, nil
}
