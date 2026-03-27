package java

import (
	"fmt"
	"strconv"
	"strings"
)

type Runtime struct {
	Major int
}

func (r Runtime) Name() string {
	return fmt.Sprintf("java:%d", r.Major)
}

func (r Runtime) BaseImage() string {
	return "eclipse-temurin:" + strconv.Itoa(r.Major) + "-jre-alpine"
}

func (r Runtime) BinaryName() string { return "server.jar" }

func (r Runtime) Entrypoint() string {
	return "#!/bin/sh\necho \"eula=true\" > /data/eula.txt 2>/dev/null || true\nexec java $ORE_JVM_FLAGS -jar /opt/ore/server.jar \"$@\"\n"
}

func (r Runtime) BinaryMode() int64 { return 0o644 }

func MajorForMC(mcVersion string) int {
	parts := strings.SplitN(mcVersion, ".", 3)
	if len(parts) < 2 {
		return 21
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return 21
	}
	switch {
	case minor >= 21:
		return 21
	case minor >= 17:
		return 17
	default:
		return 11
	}
}
