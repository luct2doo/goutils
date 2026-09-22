package paginator

import "testing"

func TestSanitizeSort(t *testing.T) {
	cases := []struct {
		sort    string
		allowed []string
		want    string
	}{
		{"", nil, "id"},
		{"created_at", nil, "created_at"},
		{"users.created_at", nil, "users.created_at"},
		{"id; DROP TABLE users", nil, "id"},
		{"id, (SELECT 1)", nil, "id"},
		{"id--", nil, "id"},
		{"id desc", nil, "id"},
		{"  status  ", nil, "status"},
		{"1=1", []string{"1=1"}, "id"}, // 白名单里的值同样要过字形校验
		{"password", []string{"id", "status"}, "id"},
		{"status", []string{"id", "status"}, "status"},
	}
	for _, c := range cases {
		if got := sanitizeSort(c.sort, c.allowed); got != c.want {
			t.Errorf("sanitizeSort(%q, %v) = %q, want %q", c.sort, c.allowed, got, c.want)
		}
	}
}

func TestNormalizeOrder(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "desc"},
		{"asc", "asc"},
		{"ASC", "asc"},
		{"Desc", "desc"},
		{"drop", "desc"},
		{"asc; DROP TABLE users", "desc"},
		{" id", "desc"},
	}
	for _, c := range cases {
		if got := normalizeOrder(c.in); got != c.want {
			t.Errorf("normalizeOrder(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
