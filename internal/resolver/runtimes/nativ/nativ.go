package nativ

type Runtime struct{}

func (r Runtime) Name() string       { return "native" }
func (r Runtime) BaseImage() string  { return "alpine:latest" }
func (r Runtime) BinaryName() string { return "server" }
func (r Runtime) Entrypoint() string { return "" }
func (r Runtime) BinaryMode() int64  { return 0o755 }
