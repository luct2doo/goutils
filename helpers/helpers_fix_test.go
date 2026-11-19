package helpers

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateSecureDigitHonorsLength(t *testing.T) {
	for _, length := range []int{1, 4, 6, 10, 16} {
		got := GenerateSecureDigit(length)
		if len(got) != length {
			t.Errorf("GenerateSecureDigit(%d) = %q, len = %d", length, got, len(got))
		}
		if strings.HasPrefix(got, "0") {
			t.Errorf("GenerateSecureDigit(%d) = %q, 首位不应为 0", length, got)
		}
	}

	for _, length := range []int{0, -3} {
		if got := GenerateSecureDigit(length); got != "" {
			t.Errorf("GenerateSecureDigit(%d) = %q, want empty", length, got)
		}
	}
}

type TrimInner struct {
	Exported string
	hidden   string
}

type TrimOuter struct {
	TrimInner
	Name string
}

func TestTrimStructSkipsUnexportedFields(t *testing.T) {
	// 回归：内嵌结构体里的未导出字段曾导致 reflect.Value.Interface panic
	o := TrimOuter{TrimInner: TrimInner{Exported: "  x  ", hidden: "  y  "}, Name: "  n  "}
	TrimStruct(&o)

	if o.Name != "n" {
		t.Errorf("Name = %q, want %q", o.Name, "n")
	}
	if o.Exported != "x" {
		t.Errorf("Exported = %q, want %q", o.Exported, "x")
	}
	if o.hidden != "  y  " {
		t.Errorf("未导出字段不应被修改，得到 %q", o.hidden)
	}
}

func TestUnixStringRoundTrip(t *testing.T) {
	now := time.Now()
	s := UnixToString(now.Unix())
	got := StringToUnix(s)
	if got != now.Unix() {
		t.Errorf("往返后时间戳不一致: want %d, got %d (%s)", now.Unix(), got, s)
	}
}

func TestCountStructFieldsNil(t *testing.T) {
	if got := CountStructFields(nil); got != 0 {
		t.Errorf("CountStructFields(nil) = %d, want 0", got)
	}
}

func TestBuildSearchMapFromGinNil(t *testing.T) {
	// 回归：nil/无效值曾导致 reflect.MethodByName panic
	if got := BuildSearchMapFromGin(nil, []string{"a"}); len(got) != 0 {
		t.Errorf("BuildSearchMapFromGin(nil) = %v, want empty", got)
	}
}

func TestLookupRejectsInvalidIP(t *testing.T) {
	p := NewIPAPIProvider("", nil)
	if got := p.Lookup("1.2.3.4/../../admin"); got != "未知位置" {
		t.Errorf("Lookup(非法 IP) = %q, want 未知位置", got)
	}
}
