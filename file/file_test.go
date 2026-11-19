package file

import (
	"strings"
	"testing"
)

func TestSanitizePathSegment(t *testing.T) {
	for _, in := range []string{"../../etc/passwd", "..", ".", "", "a/../b", `..\win`} {
		got := sanitizePathSegment(in, "anonymous")
		if got == "" {
			t.Errorf("sanitizePathSegment(%q) 返回空串", in)
		}
		if strings.Contains(got, "..") || strings.ContainsAny(got, `/\`) {
			t.Errorf("sanitizePathSegment(%q) = %q，仍包含穿越字符", in, got)
		}
	}

	if got := sanitizePathSegment("user-1", "anonymous"); got != "user-1" {
		t.Errorf("合法片段被改写: %q", got)
	}
	if got := sanitizePathSegment("a b", "anonymous"); got != "a_b" {
		t.Errorf("sanitizePathSegment(\"a b\") = %q, want a_b", got)
	}
}
