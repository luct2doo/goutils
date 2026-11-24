package verifycode

type Store interface {
	Set(id, value string) bool
	Get(id string, clear bool) string
	Verify(id, answer string, clear bool) bool
}
