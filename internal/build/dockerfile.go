package build

import (
	"fmt"

	"github.com/oreforge/ore/internal/resolver/runtimes"
)

type DockerfileOptions struct {
	Runtime       runtimes.Runtime
	ExtraArgs     string
	HealthRetries int
}

func GenerateDockerfile(opts DockerfileOptions) string {
	entrypoint := opts.Runtime.Entrypoint()
	if entrypoint != "" {
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
%s
ENTRYPOINT ["tini", "--", "/opt/ore/entrypoint.sh"]%s
`, opts.Runtime.BaseImage(), opts.Runtime.BinaryName(), opts.Runtime.BinaryName(),
		dockerHealthcheck(opts.HealthRetries), cmdLine)
}

func generateDirectDockerfile(opts DockerfileOptions) string {
	return fmt.Sprintf(`FROM %s
RUN apk add --no-cache tini
COPY %s /opt/ore/%s
COPY data/ /data/
WORKDIR /data
EXPOSE 25565
%s
ENTRYPOINT ["tini", "--", "/opt/ore/%s"]
`, opts.Runtime.BaseImage(), opts.Runtime.BinaryName(), opts.Runtime.BinaryName(),
		dockerHealthcheck(opts.HealthRetries), opts.Runtime.BinaryName())
}

func dockerHealthcheck(retries int) string {
	return fmt.Sprintf(
		`HEALTHCHECK --interval=2s --timeout=2s --start-period=5s --retries=%d \
    CMD nc -z localhost 25565 || exit 1`,
		retries,
	)
}
