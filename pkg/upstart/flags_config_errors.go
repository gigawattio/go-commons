package upstart

import (
	"errors"
)

var (
	IllegalBindToError                  = errors.New("upstart/config.BaseConfig: Illegal nil value received for `bindTo'")
	InvalidFlagsInstallUninstallError   = errors.New("-install flag must not be accompanied by -uninstall flag")
	InvalidFlagsMissingServiceUserError = errors.New(`-user flag must be specified if -install flag is set`)
)
