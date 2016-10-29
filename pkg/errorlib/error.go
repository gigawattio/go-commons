package errorlib

import (
	"fmt"
	"runtime"
)

// Errorf constructs informative errors which include helpful contextual
// information.
func Error(detail interface{}) error {
	if detail == nil {
		return nil
	}
	pc, fn, line, _ := runtime.Caller(1)
	err := fmt.Errorf("%s[%s:%d] %v", runtime.FuncForPC(pc).Name(), fn, line, detail)
	return err
}

// Errorf is just like `Error` with the addition of string formatting.
func Errorf(format string, a ...interface{}) error {
	detail := fmt.Sprintf(format, a...)
	pc, fn, line, _ := runtime.Caller(1)
	err := fmt.Errorf("%s[%s:%d] %v", runtime.FuncForPC(pc).Name(), fn, line, detail)
	return err
}
