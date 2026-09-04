package tunnels

import (
	"errors"
	"testing"
)

func TestLoadRefusesBadTokens(t *testing.T) {
	cases := []struct {
		name  string
		token string
		want  error
	}{
		{"absent", "", ErrNoToken},
		{"whitespace only", "   ", ErrNoToken},
		{"not a tunnels token", "abc123", ErrNoToken},
		{"team service token", "tnlt_something", ErrTeamToken},
		{"a JWT pasted by mistake", "eyJhbGciOiJIUzI1NiJ9.x.y", ErrNoToken},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("TUNNELS_TOKEN", c.token)
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			_, err := Load()
			if !errors.Is(err, c.want) {
				t.Fatalf("err=%v want=%v", err, c.want)
			}
		})
	}
}

func TestTeamTokenBeatsTheGenericCheck(t *testing.T) {
	t.Setenv("TUNNELS_TOKEN", "tnlt_abc")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := Load()
	if errors.Is(err, ErrNoToken) {
		t.Fatal("got ErrNoToken want ErrTeamToken")
	}
}

func TestLoadAcceptsAPersonalToken(t *testing.T) {
	t.Setenv("TUNNELS_TOKEN", "tnl_abc")
	t.Setenv("TUNNELS_API", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != "tnl_abc" {
		t.Fatalf("token=%q", c.Token)
	}
	if c.Base != DefaultBase {
		t.Fatalf("base=%q want=%q", c.Base, DefaultBase)
	}
}

func TestEnvironmentOverridesTheBase(t *testing.T) {
	t.Setenv("TUNNELS_TOKEN", "tnl_abc")
	t.Setenv("TUNNELS_API", "http://localhost:28081")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Base != "http://localhost:28081" {
		t.Fatalf("base=%q", c.Base)
	}
}
