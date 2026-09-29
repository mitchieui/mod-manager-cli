package bepinex

import (
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mmcli/internal/config"
)

func TestInstallUsesNativeRuntimeLayout(t *testing.T) {
	root := t.TempDir()
	paths := config.Paths{ValheimDir: filepath.Join(root, "Valheim"), ProfilesDir: filepath.Join(root, "profiles")}
	archive := filepath.Join(root, "bepinex.zip")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, name := range []string{"BepInEx/core/BepInEx.Preloader.dll", "BepInEx/config/BepInEx.cfg", "winhttp.dll", "doorstop_config.ini"} {
		w, err := zw.Create("BepInExPack_Valheim/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Install(paths, archive); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"winhttp.dll", "doorstop_config.ini"} {
		if _, err := os.Stat(filepath.Join(paths.ValheimDir, name)); err != nil {
			t.Fatal(err)
		}
	}
	base := paths.BepInExDir()
	if runtime.GOOS == "windows" {
		base = filepath.Join(paths.ProfileDir("default"), "BepInEx")
	}
	for _, name := range []string{"core/BepInEx.Preloader.dll", "config/BepInEx.cfg"} {
		if _, err := os.Stat(filepath.Join(base, name)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPatchRunScriptUsesSteamBundleName(t *testing.T) {
	content := `#!/bin/sh
executable_name="valheim.x86_64"
exec "$executable_path" $rest_args
`

	content = replaceScriptVar(content, "executable_name", "Valheim.app")
	content = replaceExecWithArchWrapper(content)

	if !strings.Contains(content, `executable_name="Valheim.app"`) {
		t.Fatalf("patched script did not use Valheim.app:\n%s", content)
	}
	if !strings.Contains(content, "arch -x86_64 zsh -c") {
		t.Fatalf("patched script did not include Rosetta wrapper:\n%s", content)
	}
}
