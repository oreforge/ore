package spec

import (
	"crypto/sha256"
	"fmt"
	"hash"
	"sort"
)

func ServerHash(srv *Server, imageTag string) string {
	h := sha256.New()
	hashField(h, imageTag)
	hashField(h, srv.Memory)
	hashField(h, srv.CPU)
	hashList(h, srv.Ports)
	hashEnv(h, srv.Env)
	hashVolumes(h, srv.Volumes)
	hashHealthCheck(h, srv.HealthCheck)
	return fmt.Sprintf("%x", h.Sum(nil))[:12]
}

func ServiceHash(svc *Service) string {
	h := sha256.New()
	hashField(h, svc.Image)
	hashList(h, svc.Ports)
	hashEnv(h, svc.Env)
	hashVolumes(h, svc.Volumes)
	hashHealthCheck(h, svc.HealthCheck)
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

func hashVolumes(h hash.Hash, volumes []Volume) {
	for _, v := range volumes {
		h.Write([]byte(v.Name + ":" + v.Target))
		h.Write([]byte{0})
	}
}

func hashHealthCheck(h hash.Hash, hc *HealthCheck) {
	if hc == nil {
		h.Write([]byte{0})
		return
	}
	if hc.Disabled {
		h.Write([]byte("disabled"))
		h.Write([]byte{0})
		return
	}
	hashField(h, hc.Cmd)
	hashField(h, hc.Interval.String())
	hashField(h, hc.Timeout.String())
	hashField(h, hc.StartPeriod.String())
	hashField(h, fmt.Sprintf("%d", hc.Retries))
}
