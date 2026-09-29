//go:build windows

package platform

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestWindowsConfigDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	got, err := ConfigDir()
	if err != nil || got != filepath.Join(dir, "mmcli") {
		t.Fatalf("ConfigDir: %q, %v", got, err)
	}
	t.Setenv("APPDATA", "")
	if _, err := ConfigDir(); err == nil {
		t.Fatal("expected missing APPDATA error")
	}
}

func TestParseSteamRegistryPaths(t *testing.T) {
	for _, key := range []string{"SteamPath", "InstallPath"} {
		value := "C:\\Program Files (x86)\\Steam"
		out := "HKEY_CURRENT_USER\\Software\\Valve\\Steam\r\n    " + key + "    REG_SZ    " + value + "\r\n"
		if got := parseRegistryValue(out, key); got != value {
			t.Fatalf("got %q want %q", got, value)
		}
	}
}

func TestSteamLibrariesAndLaunchTarget(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "steamapps"), 0755); err != nil {
		t.Fatal(err)
	}
	data := `"libraryfolders" { "1" { "path" "D:\\SteamLibrary" } "2" { "path" "d:\\steamlibrary" } }`
	if err := os.WriteFile(filepath.Join(root, "steamapps", "libraryfolders.vdf"), []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	if got, want := steamLibraries(root), []string{root, `D:\SteamLibrary`}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if got := GameLaunchTarget(root); got != filepath.Join(root, "valheim.exe") {
		t.Fatalf("wrong launch target %s", got)
	}
}
