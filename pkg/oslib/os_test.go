package oslib

import (
	"os"
	"strings"
	"testing"
)

func Test_OsPath(t *testing.T) {
	if actual := OsPath(os.Getenv("HOME"), ".ssh", ".id_rsa"); !strings.Contains(actual, PathSep) {
		t.Errorf("Expected path=%q to contain a PathSep=%q", actual, PathSep)
	}
}
