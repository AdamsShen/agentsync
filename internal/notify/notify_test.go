package notify

import (
	"strings"
	"testing"
)

func TestDarwinScript(t *testing.T) {
	s := darwinScript("标题", "内容")
	if !strings.HasPrefix(s, "display notification ") {
		t.Fatalf("脚本应以 display notification 开头: %q", s)
	}
	if !strings.Contains(s, "with title ") {
		t.Fatalf("脚本应含 with title: %q", s)
	}
}

// 内容含双引号时应被转义，避免 AppleScript 语法错/注入
func TestDarwinScriptEscapesQuotes(t *testing.T) {
	s := darwinScript("t", `he said "hi"`)
	if strings.Contains(s, `said "hi"`) {
		t.Fatalf("双引号未转义: %q", s)
	}
}
