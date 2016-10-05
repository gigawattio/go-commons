package upstart

import (
	"bytes"
	"text/template"
)

const (
	defaultInstallBinPath = "/usr/local/bin"
	defaultUser           = "ubuntu"
)

var (
	upstartTemplate = template.Must(template.New("upstart").Parse(
		`#!upstart
description "{{.ServiceName}}"

env USER='{{.User}}'
env PID=/var/run/{{.ServiceName}}.pid
env LOG_DIR=/var/log/gigawatt
env LOG=/var/log/gigawatt/{{.ServiceName}}.log

start on (local-filesystems and net-device-up IFACE!=lo)
stop on [!12345]

respawn

console log

pre-start script
    mkdir -p /var/run
end script

script
    test -d $LOG_DIR || mkdir -p $LOG_DIR
    chown -R $USER:$USER $LOG_DIR
    echo $$ > $PID
    exec sudo -H -u $USER bash -c '[[ ! -f /etc/default/{{.ServiceName}} ]] || . /etc/default/{{.ServiceName}} && {{.ServiceBinPath}}{{if gt (len .Args) 0}} {{.Args}}{{end}}' 2>&1 | tee -a ${LOG}{{if gt (len .PipedCommand) 0}} | {{.PipedCommand}}{{end}}
end script

post-stop script
    rm -f $PID
end script
`))
)

type UpstartConfig struct {
	ServiceName         string
	Args                string
	PipedCommand        string
	InstallBinPath      string
	UpstartConfFilePath string // e.g. /etc/init/{{ServiceName}}.
	InitSymlinkPath     string // e.g. /etc/init.d/{{ServiceName}}, required for service-name tab auto-complete to work.
	User                string
}

func DefaultConfig(serviceName string) UpstartConfig {
	config := UpstartConfig{
		ServiceName:         serviceName,
		InstallBinPath:      defaultInstallBinPath,
		UpstartConfFilePath: "/etc/init/" + serviceName + ".conf",
		InitSymlinkPath:     "/etc/init.d/" + serviceName,
		User:                defaultUser,
	}
	return config
}

func (config UpstartConfig) ServiceBinPath() string {
	serviceBinPath := config.InstallBinPath + "/" + config.ServiceName
	return serviceBinPath
}

func render(config UpstartConfig) ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := upstartTemplate.Execute(buf, config); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

//sudo -E -u $USER bash -c "~${USER}/go/bin/logger -application {{.LoggerAppId}} -process {{.LoggerProcessName}}{{if gt (len .LoggerHosts) 0}} -hosts '{{.LoggerHosts}}'{{end}}
