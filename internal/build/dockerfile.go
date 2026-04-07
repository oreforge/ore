package build

import (
	"fmt"
	"time"

	"github.com/oreforge/ore/internal/software"
	"github.com/oreforge/ore/internal/spec"
)

type DockerfileOptions struct {
	Runtime     software.Runtime
	ExtraArgs   string
	HealthCheck *spec.HealthCheck
}

func GenerateDockerfile(opts DockerfileOptions) string {
	if opts.Runtime.Entrypoint != "" {
		return generateEntrypointDockerfile(opts)
	}
	return generateDirectDockerfile(opts)
}

func generateEntrypointDockerfile(opts DockerfileOptions) string {
	cmdLine := ""
	if opts.ExtraArgs != "" {
		cmdLine = fmt.Sprintf("\nCMD [%q]", opts.ExtraArgs)
	}

	return fmt.Sprintf(`FROM %s
RUN apk add --no-cache tini
COPY %s /opt/ore/%s
COPY entrypoint.sh /opt/ore/entrypoint.sh
COPY data/ /data/
WORKDIR /data
EXPOSE 25565
%sENTRYPOINT ["tini", "-s", "--", "/opt/ore/entrypoint.sh"]%s
`, opts.Runtime.BaseImage, opts.Runtime.BinaryName, opts.Runtime.BinaryName,
		dockerHealthcheck(opts.HealthCheck), cmdLine)
}

func generateDirectDockerfile(opts DockerfileOptions) string {
	return fmt.Sprintf(`FROM %s
RUN apk add --no-cache tini
COPY %s /opt/ore/%s
COPY data/ /data/
WORKDIR /data
EXPOSE 25565
%sENTRYPOINT ["tini", "-s", "--", "/opt/ore/%s"]
`, opts.Runtime.BaseImage, opts.Runtime.BinaryName, opts.Runtime.BinaryName,
		dockerHealthcheck(opts.HealthCheck), opts.Runtime.BinaryName)
}

func dockerHealthcheck(hc *spec.HealthCheck) string {
	if hc == nil || hc.Disabled || hc.Cmd == "" {
		return ""
	}

	interval := hc.Interval
	if interval == 0 {
		interval = 2 * time.Second
	}
	timeout := hc.Timeout
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	startPeriod := hc.StartPeriod
	if startPeriod == 0 {
		startPeriod = 5 * time.Second
	}
	retries := hc.Retries
	if retries == 0 {
		retries = 3
	}

	return fmt.Sprintf(`HEALTHCHECK --interval=%s --timeout=%s --start-period=%s --retries=%d \
    CMD %s || exit 1
`, interval, timeout, startPeriod, retries, hc.Cmd)
}
