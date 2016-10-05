// +build !linux

package upstart

import (
	"errors"
)

func InstallService(config UpstartConfig) error {
	return errors.New("service installation is only supported for linux")
}
func UninstallService(config UpstartConfig) error {
	return errors.New("service uninstallation is only supported for linux")
}
