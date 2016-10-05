package testlib

import (
	"fmt"
	"regexp"
	"runtime"
)

// testExpr1 is the regular expression used to extract the name of a currently running test.
// NB: Compatible with new go 1.7 stack trace format as well as the legacy format.
const testExpr1 = `\.(Test[^a-z][^\(]+)\([^)]+\)\n[^\n]+\n[ \t]*testing\.tRunner\([^\)]*\)\n[ \t]*[^\n]+\n[ \t]*created by testing\.(?:\(\*T\)\.Run|RunTests)\n`

const testExpr2 = `/[^/]+_test\.go:[1-9][0-9]* \+0x[0-9a-f]+\n[ \t]*main\.init\(\)\n[ \t]*.+_test/_testmain.go:[1-9][0-9]* \+0x[0-9a-f]`

// byteCountAllocationLimit sets the limit for the amount of bytes allocated
// while analyzing Stack traces.
// If this quantity is exceeded, the `Stack` function will panic.
const byteCountAllocationLimit = 1048576
const initialAllocation = 8192

// Stack attempts to safely ensure that the entire Stack is captured.
func Stack() []byte {
	allocateNumBytes := initialAllocation
	// Ensure the entire Stack is captured.
	for {
		buf := make([]byte, allocateNumBytes) // Must to be large enough to fit the entire trace reliably.
		numCapturedBytes := runtime.Stack(buf, false)
		// Check for all allowed bytes having been consumed.
		if numCapturedBytes == allocateNumBytes {
			// Grow allocation size.
			allocateNumBytes = allocateNumBytes * 2
			// Verify it hasn't gone beyond tolerance.
			if allocateNumBytes > byteCountAllocationLimit {
				panic(fmt.Sprintf("Stack() allocateNumBytes has grown beyond the max allowed size (current=%v limit=%v)", allocateNumBytes, byteCountAllocationLimit))
			}
			continue // Try again with larger allocation size.
		}
		return buf
	}
}

// CurrentRunningTest obtains the name of the currently running unit test.
// Use this with care, if invoked from an `init()` function, it WILL  panic.
//
// For example, given the following Stack:
//
//	apiserver_test.go:74: ok
//	apiserver_test.go:69: 424
//	apiserver_test.go:70: goroutine 7 [running]:
//		gigawatt-common/apiserver.tj(0xc208058090)
//			/Users/jay/go/src/gigawatt-common/apiserver/apiserver_test.go:69 +0x7a
//		gigawatt-common/apiserver.Test_Jay(0xc208058090)
//			/Users/jay/go/src/gigawatt-common/apiserver/apiserver_test.go:75 +0xf9
//		testing.tRunner(0xc208058090, 0x6de870)
//			/usr/local/go/src/testing/testing.go:447 +0xbf
//		created by testing.RunTests
//			/usr/local/go/src/testing/testing.go:555 +0xa8b
//
// CurrentRunningTest() will return the string "Test_Jay".
func CurrentRunningTest() string {
	st := Stack()
	expr := regexp.MustCompile(testExpr1)
	submatches := expr.FindSubmatch(st)
	if len(submatches) < 2 {
		panic(fmt.Sprintf("CurrentRunningTest() failed to extract current running test name from Stack:\n%s", string(st)))
	}
	fnName := string(submatches[1])
	return fnName
}

// IsRunningTests returns true if a unit-test is being run right now.
func IsRunningTests() bool {
	st := Stack()
	expr := regexp.MustCompile(testExpr1)
	submatches := expr.FindSubmatch(st)
	if len(submatches) < 2 {
		// Search for evidence of init phase of testing.
		if regexp.MustCompile(testExpr2).Match(st) {
			return true
		}
		return false
	}
	return true
}
