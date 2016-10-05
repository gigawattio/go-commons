package testlib

import (
	"fmt"
	"runtime"
	"testing"
)

const (
	SkipUnlessOSIllegalInvocation = "SkipUnlessOS invoked with empty `allowedOss' array"
)

// SkipUnlessOS is a common facility for skipping a test when the OS doesn't match one or more values.
func SkipUnlessOS(t *testing.T, allowedOSs ...string) {
	if len(allowedOSs) == 0 {
		panic(SkipUnlessOSIllegalInvocation)
	}
	match := false
	for _, allowedOS := range allowedOSs {
		if runtime.GOOS == allowedOS {
			match = true
			break
		}
	}
	if !match {
		t.Skipf(skipUnlessOsNotice(allowedOSs))
	}
}

// skipNotice provides the skip message.
func skipUnlessOsNotice(allowedOSs []string) string {
	var allowedMessage string
	if len(allowedOSs) == 1 {
		allowedMessage = fmt.Sprintf("=`%s'", allowedOSs[0])
	} else {
		allowedMessage = fmt.Sprintf(" be one of: %s", allowedOSs)
	}
	notice := fmt.Sprintf("Notice: Skipping test which requires OS%s, current OS is `%s'", allowedMessage, runtime.GOOS)
	return notice
}
