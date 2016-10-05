package upstart

import (
	"flag"
	"testing"

	"github.com/gigawattio/go-commons/pkg/testlib"
)

// TestFlagsConfigValidationBindTo ensures that passing nil to `Validate()'
// produces the expected IllegalBindToError.
func TestFlagsConfigValidationBindTo(t *testing.T) {
	config := NewFlagsConfig(BaseFlagSet(testlib.CurrentRunningTest()), []string{})
	if expected, actual := IllegalBindToError, config.Validate(nil); actual != expected {
		t.Fatalf("Expected Validate(nil)=%s but actual=%s", expected, actual)
	}
}

func TestFlagsConfigScenarios(t *testing.T) {
	type (
		Validatable interface {
			Validate(bindTo interface{}) error
		}

		ConfigExtra struct {
			*FlagsConfig
			Extra bool `flag:"extra"`
		}
	)

	testCases := []struct {
		name          string
		args          []string
		expectedError error
		configFunc    func(flagSet *flag.FlagSet, args []string) Validatable // Custom config struct instance provider.  If nil, *FlagsConfig will be used.
		callbackFunc  func(config interface{})                               // Callback which is invoked with the config struct instance.
	}{
		{
			name:          "expect-InvalidFlagsInstallUninstallError",
			args:          []string{"-install", "-uninstall"},
			expectedError: InvalidFlagsInstallUninstallError,
		},
		{
			name:          "expect-InvalidFlagsMissingServiceUserError",
			args:          []string{"-install"},
			expectedError: InvalidFlagsMissingServiceUserError,
		},
		{
			name:          "basic-test",
			args:          []string{"-install", "-user", "jay"},
			expectedError: nil,
			callbackFunc: func(configIface interface{}) {
				config := configIface.(*FlagsConfig)
				if expected, actual := false, config.Uninstall; actual != expected {
					t.Logf("config=%+v", *config)
					t.Errorf("Expected config.Uninstall=%v but actual=%v", expected, actual)
				}
				if expected, actual := true, config.Install; actual != expected {
					t.Logf("config=%+v", config)
					t.Errorf("Expected config.Install=%v but actual=%v", expected, actual)
				}
				if expected, actual := "jay", config.ServiceUser; actual != expected {
					t.Logf("config=%+v", config)
					t.Errorf("Expected config.ServiceUser=%v but actual=%v", expected, actual)
				}
			},
		},
		{
			name:          "extra-flag-test",
			args:          []string{"-extra=true", "-install", "-user=jay"},
			expectedError: nil,
			configFunc: func(flagSet *flag.FlagSet, args []string) Validatable {
				flagSet.Bool("extra", false, "usage")
				config := &ConfigExtra{
					FlagsConfig: NewFlagsConfig(flagSet, args),
				}
				return config
			},
			callbackFunc: func(configIface interface{}) {
				config := configIface.(*ConfigExtra)
				if expected, actual := true, config.Extra; actual != expected {
					t.Logf("config=%+v", *config)
					t.Errorf("Expected config.Extra=%v but actual=%v", expected, actual)
				}
				if expected, actual := true, config.Install; actual != expected {
					t.Logf("config=%+v", config)
					t.Errorf("Expected config.Install=%v but actual=%v", expected, actual)
				}
				if expected, actual := "jay", config.ServiceUser; actual != expected {
					t.Logf("config=%+v", config)
					t.Errorf("Expected config.ServiceUser=%v but actual=%v", expected, actual)
				}
			},
		},
	}

	for i, testCase := range testCases {
		if testCase.configFunc == nil {
			// Default config provider.
			testCase.configFunc = func(flagSet *flag.FlagSet, args []string) Validatable {
				return NewFlagsConfig(flagSet, args)
			}
		}

		var (
			flagSet = BaseFlagSet(testCase.name)
			config  = testCase.configFunc(flagSet, testCase.args)
		)

		if actual, expected := config.Validate(config), testCase.expectedError; actual != expected {
			t.Logf("[i=%v] testCase=%+v", i, testCase)
			t.Fatalf("[i=%v] Expected error=%s but actual=%s", i, expected, actual)
		}

		if testCase.callbackFunc != nil {
			testCase.callbackFunc(config)
		}
	}
}
