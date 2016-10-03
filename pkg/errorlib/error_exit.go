package errorlib

import (
	"os"
)

func ErrorExit(reason error, statusCode int) {
	if reason != nil {
		os.Stderr.WriteString("error: " + reason.Error() + "\n")
		os.Exit(statusCode)
	}
}
