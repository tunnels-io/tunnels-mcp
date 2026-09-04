package tunnels

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const DefaultBase = "https://tunnels.io"

type Config struct {
	Token string
	Base  string
	CLI   string
}

var ErrNoToken = errors.New("no tunnels.io token")

var ErrTeamToken = errors.New("team service token")

type fileConfig struct {
	Token string `json:"token"`
	Base  string `json:"base,omitempty"`
	CLI   string `json:"cli,omitempty"`
}

func Load() (Config, error) {
	c := Config{
		Token: strings.TrimSpace(os.Getenv("TUNNELS_TOKEN")),
		Base:  strings.TrimSpace(os.Getenv("TUNNELS_API")),
		CLI:   strings.TrimSpace(os.Getenv("TUNNELS_CLI")),
	}

	if c.Token == "" || c.Base == "" || c.CLI == "" {
		if fc, ok := readFile(); ok {
			if c.Token == "" {
				c.Token = strings.TrimSpace(fc.Token)
			}
			if c.Base == "" {
				c.Base = strings.TrimSpace(fc.Base)
			}
			if c.CLI == "" {
				c.CLI = strings.TrimSpace(fc.CLI)
			}
		}
	}
	if c.Base == "" {
		c.Base = DefaultBase
	}
	if c.Token == "" {
		return c, ErrNoToken
	}
	if strings.HasPrefix(c.Token, TeamTokenPrefix) {
		return c, ErrTeamToken
	}
	if !strings.HasPrefix(c.Token, TokenPrefix) {
		return c, ErrNoToken
	}
	return c, nil
}

func ConfigPath() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "tunnels", "mcp.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "tunnels", "mcp.json")
}

func readFile() (fileConfig, bool) {
	p := ConfigPath()
	if p == "" {
		return fileConfig{}, false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return fileConfig{}, false
	}
	var fc fileConfig
	if err := json.Unmarshal(b, &fc); err != nil {
		return fileConfig{}, false
	}
	return fc, true
}
