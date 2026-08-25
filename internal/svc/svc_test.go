package svc

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemdUnitContent(t *testing.T) {
	c := systemdUnitContent()
	for _, want := range []string{"ExecStart=", " daemon", "Restart=on-failure", "WantedBy=default.target", "[Install]"} {
		if !strings.Contains(c, want) {
			t.Fatalf("systemd unit 缺 %q，实际:\n%s", want, c)
		}
	}
}

func TestSystemdUnitPath(t *testing.T) {
	p := systemdUnitPath()
	if !strings.HasSuffix(p, filepath.Join(".config", "systemd", "user", "agentsync.service")) {
		t.Fatalf("unit 路径错误: %s", p)
	}
}
