package testlib

import (
	"fmt"
	"testing"
)

func Test_SkipEmptyOSsPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			recoveredMessage := fmt.Sprintf("%s", r)
			if recoveredMessage != SkipUnlessOSIllegalInvocation {
				t.Error(`Recovered message "%s" did not match expected message "%s"`, recoveredMessage, SkipUnlessOSIllegalInvocation)
			} else {
				t.Log("Successfully generated a panic by invoking `SkipUnlessOS' with empty allowed OS's")
			}
		} else {
			t.Error("Expected `SkipUnlessOS' to panic when invoked with no allowed OS's, but no panic occurred")
		}
	}()
	SkipUnlessOS(t)
}
