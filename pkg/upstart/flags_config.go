package upstart

import (
	"flag"
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/mreiferson/go-options"
)

// FlagsConfig is a common representation of the fundamental set of options for
// upstart service management.  Intended to help reduce repetitive command-line
// flags handling.
//
// NB: A flag for "config" is also used, even though it is not defined as a
// field in FlagsConfig.
//
// NB: This uses github.com/BurntSushi/toml and github.com/mreiferson/go-options
// packages for flag management.
type FlagsConfig struct {
	Install     bool   `flag:"install"`
	CustomPipe  string `flag:"install-with-custom-pipe"`
	Uninstall   bool   `flag:"uninstall"`
	ServiceUser string `flag:"user"`
	ServiceArgs string // Automatically populated within `Validate()'; used for installing system service.

	args    []string
	flagSet *flag.FlagSet
	tomlMap map[string]interface{}
}

// New creates and returns a new instance of FlagsConfig.
//
// NB: In the common case the value of `args' should be `os.Args[1:]'.
func NewFlagsConfig(flagSet *flag.FlagSet, args []string) *FlagsConfig {
	config := &FlagsConfig{
		args:    args,
		flagSet: flagSet,
	}
	return config
}

func (config *FlagsConfig) ConfigMap() map[string]interface{} { return copyMap(config.tomlMap) } // NB: Part of the gigawatt-logger/receiver.OptionsProvider interface.
func (config *FlagsConfig) FlagSet() *flag.FlagSet            { return config.flagSet }          // NB: Part of the gigawatt-logger/receiver.OptionsProvider interface.
func (config *FlagsConfig) Args() []string                    { return config.args }

// Validate takes a `bindTo' parameter which *MUST* be a pointer to the topmost
// Config struct instance (assuming FlagsConfig contained as an embedded type).
// This ensures that the flag bindings get applied to all fields.
//
// If you override the Validate method, don't forget to invoke base validation.
//
// Example usage:
//
//     myConfig.Validate(myConfig)
//
// or
//
//     myConfig.FlagsConfig.Validate(myConfig).
func (config *FlagsConfig) Validate(bindTo interface{}) error {
	if bindTo == nil {
		return IllegalBindToError
	}
	if err := config.flagSet.Parse(config.args); err != nil {
		return err
	}
	if configFile := config.flagSet.Lookup("config").Value.String(); configFile != "" {
		if _, err := toml.DecodeFile(configFile, &config.tomlMap); err != nil { // NB: `_` contains TOML metadata.
			return err
		}
	}

	options.Resolve(bindTo, config.flagSet, config.tomlMap)

	if config.Install && config.Uninstall {
		return InvalidFlagsInstallUninstallError
	}
	if config.Install && len(config.ServiceUser) == 0 {
		return InvalidFlagsMissingServiceUserError
	}
	if config.Install {
		commandLineArgsMap := map[string]struct{}{}
		for _, arg := range config.args {
			if strings.HasPrefix(arg, "-") {
				arg = strings.Split(arg, "=")[0]
				if len(arg) > 0 {
					commandLineArgsMap[arg[1:]] = struct{}{}
				}
			}
		}
		config.ServiceArgs = ""
		config.flagSet.VisitAll(func(f *flag.Flag) {
			if _, ok := commandLineArgsMap[f.Name]; ok && f.Name != "install" && f.Name != "uninstall" && f.Name != "user" && f.Name != "install-with-custom-pipe" {
				config.ServiceArgs = strings.TrimSpace(fmt.Sprintf(`%v -%v=%v`, config.ServiceArgs, f.Name, f.Value.String()))
			}
		})
	}

	return nil
}

func (config *FlagsConfig) InstallService(serviceName string) error {
	serviceConfig := DefaultConfig(serviceName)
	serviceConfig.User = config.ServiceUser
	serviceConfig.Args = config.ServiceArgs
	serviceConfig.PipedCommand = config.CustomPipe

	if err := InstallService(serviceConfig); err != nil {
		return err
	}
	return nil
}

func (config *FlagsConfig) UninstallService(serviceName string) error {
	if err := UninstallService(DefaultConfig(serviceName)); err != nil {
		return err
	}
	return nil
}

func copyMap(src map[string]interface{}) map[string]interface{} {
	dst := map[string]interface{}{}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
