package client

import (
	"testing"
)

func TestHello(t *testing.T) {
	got := Hello()
	if got != "Hello" {
		t.Errorf("Helo() = %s; want \"Hello\"", got)
	}
}
