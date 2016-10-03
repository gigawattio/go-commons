package errorlib

import (
	"bytes"
	"errors"
	"fmt"
)

// Merge merges a slice of errors into a single error.
func Merge(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	var buf bytes.Buffer
	numErrors := 0
	for _, err := range errs {
		if err == nil {
			continue
		}
		if numErrors > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(err.Error())
		numErrors++
	}
	if numErrors == 0 {
		return nil
	} else if numErrors == 1 {
		return errors.New(buf.String())
	}
	message := fmt.Sprintf("%v errors: %s", numErrors, buf.String())
	return errors.New(message)
}
