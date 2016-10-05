package upstart

import (
	"flag"
	"fmt"
	"os"
)

// BaseFlagSet provides a *flag.FlagSet which corresponds with BaseConfig.
// Extend with additional flags to suit needs.
func BaseFlagSet(name string) *flag.FlagSet {
	flagSet := flag.NewFlagSet(name, flag.ExitOnError)

	flagSet.String("config", "", "path to .toml config file")

	// Installation flags.
	flagSet.Bool("install", false, "install the logserver service")
	flagSet.String("install-with-custom-pipe", "", `when installing service: pipe flux-capacitor output to specified additional shell command; e.g. 'sudo -E -u $USER bash -c "~${USER}/go/bin/some-binary -application 1-1-my-app -process $(hostname)' would result in an upstart definition with 'flux-capacitor -flags | sudo -E -u $USER bash -c "~${USER}/go/bin/logger -application 1-1-my-app -process $(hostname)'. (optional)`)
	flagSet.String("user", "", "specify the name of user the service will be run as (required when installing system service)")
	flagSet.Bool("uninstall", false, "uninstall the logserver service")

	flagSet.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %v:\n", name)
		flagSet.SetOutput(os.Stderr)
		flagSet.PrintDefaults()
	}

	return flagSet
}
