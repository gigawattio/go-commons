// +build linux

package upstart

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"

	log "github.com/Sirupsen/logrus"
)

type ErrorProducer func(config UpstartConfig) error // Installation steps are each [possible] error producers.

var (
	installSteps = []ErrorProducer{
		checkOs,
		checkIfRoot,
		checkIfServiceUserExists,
		ignoreFailure(stopService, destroyService, removeBinary),
		copyBinary,
		createService,
		startService,
	}

	uninstallSteps = []ErrorProducer{
		checkOs,
		checkIfRoot,
		ignoreFailure(stopService, destroyService, removeBinary),
	}
)

var (
	MustRunAsRootToInstall = errors.New("must be run as root to install system service")
	UnsupportedOsError     = errors.New("unsupported operating system (must be ubuntu)")
)

func InstallService(config UpstartConfig) error {
	log.Info("installing service..")
	for i, fn := range installSteps {
		if err := fn(config); err != nil {
			return fmt.Errorf("during step %v/%v: %v: %s", i+1, len(installSteps), FunctionName(fn), err)
		}
	}
	log.Info("service successfully installed")
	return nil
}
func UninstallService(config UpstartConfig) error {
	log.Info("uninstalling service..")
	for i, fn := range uninstallSteps {
		if err := fn(config); err != nil {
			return fmt.Errorf("during step %v/%v: %v: %s", i+1, len(uninstallSteps), FunctionName(fn), err)
		}
	}
	log.Info("service successfully uninstalled")
	return nil
}

func copyBinary(config UpstartConfig) error {
	output, err := exec.Command("cp", os.Args[0], config.ServiceBinPath()).CombinedOutput()
	if err != nil {
		return fmt.Errorf("copying binary from src=%v to destination=%v: %s, output=%v", os.Args[0], config.ServiceBinPath(), err, string(output))
	}
	log.Infof("✔ copied binary to %v", config.ServiceBinPath())
	return nil
}
func removeBinary(config UpstartConfig) error {
	exists, err := PathExists(config.ServiceBinPath())
	if err != nil {
		return fmt.Errorf("checking if config.ServiceBinPath=%v already exists: %s", config.ServiceBinPath(), err)
	}
	if exists {
		if err := os.RemoveAll(config.ServiceBinPath()); err != nil {
			return fmt.Errorf("removing config.ServiceBinPath=%v: %s", config.ServiceBinPath(), err)
		}
	}
	log.Infof("✔ removed binary from %v", config.ServiceBinPath())
	return nil
}

func createService(config UpstartConfig) error {
	content, err := render(config)
	if err != nil {
		return fmt.Errorf("rendering upstart template: %s", err)
	}
	if err := ioutil.WriteFile(config.UpstartConfFilePath, content, os.FileMode(int(0644))); err != nil {
		return fmt.Errorf("writing UpstartConfFilePath=%v: %s", config.UpstartConfFilePath, err)
	}
	{
		exists, err := PathExists(config.InitSymlinkPath)
		if err != nil {
			return fmt.Errorf("checking if init symlink at %v already exists: %s", config.InitSymlinkPath, err)
		}
		if exists {
			if err := os.RemoveAll(config.InitSymlinkPath); err != nil {
				return fmt.Errorf("removing init symlink at %v: %s", config.InitSymlinkPath, err)
			} else {
				log.Infof("✔ unlinked init symlink from %v", config.InitSymlinkPath)
			}
		}
	}
	if err := os.Symlink(config.UpstartConfFilePath, config.InitSymlinkPath); err != nil {
		return fmt.Errorf("symlinking %v to %v: %s", config.UpstartConfFilePath, config.InitSymlinkPath, err)
	}
	log.Infof("✔ created upstart conf: %v", config.UpstartConfFilePath)
	log.Infof("✔ created init symlink: %v", config.InitSymlinkPath)
	return nil
}
func destroyService(config UpstartConfig) error {
	for name, path := range map[string]string{
		"upstart conf": config.UpstartConfFilePath,
		"init symlink": config.InitSymlinkPath,
	} {
		exists, err := PathExists(path)
		if err != nil {
			return fmt.Errorf("checking if %v at %v already exists: %s", name, path, err)
		}
		if exists {
			if err := os.RemoveAll(path); err != nil {
				return fmt.Errorf("removing %v at %v: %s", name, path, err)
			}
			log.Infof("✔ removed %v: %v", name, path)
		} else {
			log.Infof("✔ %v removal not necessary (%v doesn't exist)", name, path)
		}
	}
	return nil
}

func restartService(config UpstartConfig) error {
	output, err := exec.Command("service", config.ServiceName, "restart").CombinedOutput()
	if err != nil {
		return fmt.Errorf("restarting logserver service: %s, output=%v", err, string(output))
	}
	log.Infof("✔ %v service restarted", config.ServiceName)
	return nil
}
func startService(config UpstartConfig) error {
	output, err := exec.Command("service", config.ServiceName, "start").CombinedOutput()
	if err != nil {
		return fmt.Errorf("starting logserver service: %s, output=%v", err, string(output))
	}
	log.Infof("✔ %v service started", config.ServiceName)
	return nil
}
func stopService(config UpstartConfig) error {
	output, err := exec.Command("service", config.ServiceName, "stop").CombinedOutput()
	if err != nil {
		return fmt.Errorf("stopping logserver service: %s, output=%v", err, string(output))
	}
	log.Infof("✔ %v service stopped", config.ServiceName)
	return nil
}

func checkOs(config UpstartConfig) error {
	if runtime.GOOS != "linux" {
		return UnsupportedOsError
	}
	log.Info("✔ os check passed")
	return nil
}
func checkIfRoot(config UpstartConfig) error {
	u, err := user.Current()
	if err != nil {
		return err
	}
	if u.Uid != "0" {
		return MustRunAsRootToInstall
	}
	log.Info("✔ running as root check passed")
	return nil
}
func checkIfServiceUserExists(config UpstartConfig) error {
	passwdFileBytes, err := ioutil.ReadFile("/etc/passwd")
	if err != nil {
		return fmt.Errorf("reading /etc/passwd: %s", err)
	}
	for _, line := range strings.Split(string(passwdFileBytes), "\n") {
		if strings.HasPrefix(line, config.User+":") {
			log.Infof("✔ verified existence of service user %q", config.User)
			return nil
		}
	}
	return fmt.Errorf("no such user %q", config.User)
}

func ignoreFailure(fns ...ErrorProducer) ErrorProducer {
	return func(config UpstartConfig) error {
		for _, fn := range fns {
			if err := fn(config); err != nil {
				log.Warning("%s", err)
			}
		}
		return nil
	}
}
