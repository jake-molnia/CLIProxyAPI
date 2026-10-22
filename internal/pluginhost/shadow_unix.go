//go:build cgo && (linux || darwin || freebsd)

package pluginhost

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func removeShadowPlugin(path string) {
	if path == "" {
		return
	}
	_ = os.Remove(path)
}

func removeShadowPluginDir(dir string) {
	_ = os.RemoveAll(dir)
}

func shadowPluginProcessAlive(pid int) bool {
	// Only ESRCH proves the process is gone; any other outcome keeps the
	// directory so a live process never loses its resident plugin images.
	errSignal := syscall.Kill(pid, 0)
	return !errors.Is(errSignal, syscall.ESRCH)
}

func shadowPluginRoot() (string, error) {
	uid := os.Geteuid()
	root := filepath.Join(os.TempDir(), fmt.Sprintf("cliproxy-pluginhost-uid-%d", uid))
	if errMkdir := os.Mkdir(root, 0o700); errMkdir != nil && !errors.Is(errMkdir, os.ErrExist) {
		return "", errMkdir
	}
	info, errStat := os.Lstat(root)
	if errStat != nil {
		return "", errStat
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || !ok || stat.Uid != uint32(uid) || info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("plugin shadow root is not a private directory owned by uid %d: %s", uid, root)
	}
	return root, nil
}
