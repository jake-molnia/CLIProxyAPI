//go:build cgo && (linux || darwin || freebsd)

package pluginhost

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestShadowCopyIgnoresInaccessibleLegacySharedRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses the permission failure being reproduced")
	}
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	legacy := filepath.Join(tmp, "cliproxy-pluginhost")
	if err := os.Mkdir(legacy, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(legacy, 0o700) })
	source := filepath.Join(tmp, "plugin.so")
	if err := os.WriteFile(source, []byte("plugin artifact"), 0o600); err != nil {
		t.Fatal(err)
	}
	shadow, err := shadowCopyPlugin(pluginFile{ID: "regression", Path: source})
	if err != nil {
		t.Fatalf("shared directory blocked this user's plugin: %v", err)
	}
	data, err := os.ReadFile(shadow)
	if err != nil || string(data) != "plugin artifact" {
		t.Fatalf("shadow copy = %q, error = %v", data, err)
	}
	rootInfo, err := os.Lstat(filepath.Dir(filepath.Dir(shadow)))
	if err != nil {
		t.Fatal(err)
	}
	if stat, ok := rootInfo.Sys().(*syscall.Stat_t); !ok || stat.Uid != uint32(os.Geteuid()) || rootInfo.Mode().Perm()&0o077 != 0 {
		t.Fatal("plugin root is not private and owned by this user")
	}
}

func TestShadowPluginDirRejectsUnsafeUserRoot(t *testing.T) {
	for _, kind := range []string{"public-directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("TMPDIR", tmp)
			root := filepath.Join(tmp, fmt.Sprintf("cliproxy-pluginhost-uid-%d", os.Geteuid()))
			if kind == "symlink" {
				if err := os.Symlink(t.TempDir(), root); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(root, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := shadowPluginDir(); err == nil {
				t.Fatal("unsafe plugin root was accepted")
			}
		})
	}
}
