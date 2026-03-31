package spec

import (
	"crypto/sha256"
	"fmt"
	"hash"
	"sort"
)

func ServerConfigHash(srv *ServerSpec, imageTag string) string {
	h := sha256.New()
	hashField(h, imageTag)
	hashField(h, srv.Memory)
	hashField(h, srv.CPU)
	hashList(h, srv.Ports)
	hashList(h, srv.JVMFlags)
	hashEnv(h, srv.Env)
	hashVolumes(h, srv.Volumes)
	return fmt.Sprintf("%x", h.Sum(nil))[:12]
}

func ServiceConfigHash(svc *ServiceSpec) string {
	h := sha256.New()
	hashField(h, svc.Image)
	hashList(h, svc.Ports)
	hashEnv(h, svc.Env)
	hashVolumes(h, svc.Volumes)
	return fmt.Sprintf("%x", h.Sum(nil))[:12]
}

func hashField(h hash.Hash, s string) {
	h.Write([]byte(s))
	h.Write([]byte{0})
}

func hashList(h hash.Hash, items []string) {
	for _, item := range items {
		h.Write([]byte(item))
		h.Write([]byte{0})
	}
	h.Write([]byte{1})
}

func hashEnv(h hash.Hash, env map[string]string) {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		h.Write([]byte(k + "=" + env[k]))
		h.Write([]byte{0})
	}
	h.Write([]byte{1})
}

func hashVolumes(h hash.Hash, volumes []VolumeSpec) {
	for _, v := range volumes {
		h.Write([]byte(v.Name + ":" + v.Target))
		h.Write([]byte{0})
	}
}
