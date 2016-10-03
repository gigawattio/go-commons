package errorlib

import (
	"errors"
	"testing"
)

func Test_MergeEmpty(t *testing.T) {
	result := Merge([]error{})
	if result != nil {
		t.Error("Expected []error{} to produce `nil', but instead got: %s", result)
	}
}

func Test_MergeSingle(t *testing.T) {
	errorMessage := "error: this is a test"
	errs := []error{errors.New(errorMessage)}
	result := Merge(errs)
	if result == nil {
		t.Fatal("Found a nil result where non-nil was expected")
	}
	if result.Error() != errorMessage {
		t.Fatalf(`Expected error string="%s" but instead found "%s"`, errorMessage, result.Error())
	}
}
func Test_MergeSeveral(t *testing.T) {
	errs := []error{
		errors.New("error: this is a test"),
		errors.New("error: this is still a test"),
	}
	result := Merge(errs)
	if result == nil {
		t.Fatal("Found a nil result where non-nil was expected")
	}
	expectedMinLen := len(errs[0].Error() + errs[1].Error())
	actualLen := len(result.Error())
	if actualLen < expectedMinLen {
		t.Errorf("Result(L=%v) had a suspiciously short length compared to the input(L=%v), it was shorter than the first two error strings; input=%v output=%v", actualLen, expectedMinLen, errs, result)
	}
}

func Test_MergeMidNil(t *testing.T) {
	testCases := []struct {
		input     []error
		expectNil bool
	}{
		{[]error{}, true},
		{[]error{nil}, true},
		{[]error{nil, nil}, true},
		{[]error{nil, nil, nil}, true},
		{
			[]error{
				errors.New("first error"),
				errors.New("first error"),
				nil,
				errors.New("fourth error"),
				errors.New("fifth error"),
			},
			false,
		},
		{
			[]error{
				nil,
				nil,
				nil,
				nil,
				errors.New("third error"),
				nil,
				nil,
				nil,
				nil,
			},
			false,
		},
	}
	for _, testCase := range testCases {
		result := Merge(testCase.input)
		if testCase.expectNil && result != nil {
			t.Errorf("Found a non-nil result where non-nil was expected; input=%v result=%v", testCase.input, result)
		} else if !testCase.expectNil && result == nil {
			t.Errorf("Found a nil result where non-nil was expected; input=%v result=%v", testCase.input, result)
		}
	}
}
