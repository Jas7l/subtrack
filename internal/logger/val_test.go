package logger

import "testing"

func TestVal(t *testing.T) {
	n := int64(7)
	if Val(&n) != int64(7) {
		t.Error("value not dereferenced")
	}
	if Val[int64](nil) != nil {
		t.Error("nil pointer must log as nil")
	}
}
