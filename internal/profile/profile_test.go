package profile

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mmcli/internal/config"
)

func testPaths(t *testing.T) config.Paths {
	t.Helper()
	tmp := t.TempDir()
	valheim := filepath.Join(tmp, "Valheim")
	os.MkdirAll(filepath.Join(valheim, "BepInEx", "config"), 0755)
	os.MkdirAll(filepath.Join(valheim, "BepInEx", "plugins"), 0755)

	return config.Paths{
		ConfigDir:   filepath.Join(tmp, "config"),
		ProfilesDir: filepath.Join(tmp, "config", "profiles"),
		ValheimDir:  valheim,
	}
}

func TestCreateProfile(t *testing.T) {
	paths := testPaths(t)

	if err := Create(paths, "test"); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify directories were created
	for sub, dir := range profileDirs(paths, "test") {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("missing directory: %s", sub)
		}
	}
}

func TestCreateProfileDuplicate(t *testing.T) {
	paths := testPaths(t)

	Create(paths, "test")
	err := Create(paths, "test")
	if err == nil {
		t.Error("Create should fail for duplicate profile")
	}
}

func TestListProfiles(t *testing.T) {
	paths := testPaths(t)

	Create(paths, "alpha")
	Create(paths, "beta")

	names, err := List(paths)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(names) != 2 {
		t.Fatalf("got %d profiles, want 2", len(names))
	}

	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet["alpha"] || !nameSet["beta"] {
		t.Errorf("expected alpha and beta, got %v", names)
	}
}

func TestListProfilesEmpty(t *testing.T) {
	paths := testPaths(t)
	names, err := List(paths)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if names != nil {
		t.Errorf("expected nil for non-existent dir, got %v", names)
	}
}

func TestDeleteProfile(t *testing.T) {
	paths := testPaths(t)
	Create(paths, "todelete")

	cfg := config.Config{ActiveProfile: "other"}
	if err := Delete(paths, cfg, "todelete"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if _, err := os.Stat(paths.ProfileDir("todelete")); !os.IsNotExist(err) {
		t.Error("profile directory should be deleted")
	}
}

func TestDeleteActiveProfile(t *testing.T) {
	paths := testPaths(t)
	Create(paths, "active")

	cfg := config.Config{ActiveProfile: "active"}
	err := Delete(paths, cfg, "active")
	if err == nil {
		t.Error("Delete should refuse to delete active profile")
	}
}

func TestDeleteNonExistentProfile(t *testing.T) {
	paths := testPaths(t)
	cfg := config.Config{ActiveProfile: "other"}
	err := Delete(paths, cfg, "nonexistent")
	if err == nil {
		t.Error("Delete should fail for non-existent profile")
	}
}

func TestSwitchProfile(t *testing.T) {
	paths := testPaths(t)
	Create(paths, "first")
	os.MkdirAll(paths.ProfileCoreDir("first"), 0755)
	os.WriteFile(filepath.Join(paths.ProfileCoreDir("first"), "BepInEx.Preloader.dll"), []byte("runtime"), 0644)
	Create(paths, "second")

	cfg := &config.Config{ActiveProfile: "first"}
	if err := Switch(paths, cfg, "second"); err != nil {
		t.Fatalf("Switch failed: %v", err)
	}

	if cfg.ActiveProfile != "second" {
		t.Errorf("ActiveProfile = %q, want %q", cfg.ActiveProfile, "second")
	}

	if runtime.GOOS == "windows" {
		data, err := os.ReadFile(filepath.Join(paths.ValheimDir, "doorstop_config.ini"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "target_assembly="+filepath.Join(paths.ProfileCoreDir("second"), "BepInEx.Preloader.dll")) {
			t.Fatalf("wrong Windows profile target: %s", data)
		}
		if _, err := os.Lstat(paths.BepInExPluginsDir()); err != nil {
			t.Fatal(err)
		}
		return
	}
	// Verify symlinks point to the right profile
	pluginsLink := paths.BepInExPluginsDir()
	target, err := os.Readlink(pluginsLink)
	if err != nil {
		t.Fatalf("Readlink failed: %v", err)
	}
	if target != paths.ProfilePluginsDir("second") {
		t.Errorf("plugins symlink = %q, want %q", target, paths.ProfilePluginsDir("second"))
	}
}

func TestSwitchNonExistentProfile(t *testing.T) {
	paths := testPaths(t)
	cfg := &config.Config{ActiveProfile: "first"}
	err := Switch(paths, cfg, "nonexistent")
	if err == nil {
		t.Error("Switch should fail for non-existent profile")
	}
}

func TestCreateProfileCopiesBepInExCfg(t *testing.T) {
	paths := testPaths(t)

	// Create a BepInEx.cfg in the config dir
	bepCfg := filepath.Join(paths.BepInExConfigDir(), "BepInEx.cfg")
	if runtime.GOOS == "windows" {
		Create(paths, "template")
		os.WriteFile(filepath.Join(paths.ProfileCoreDir("template"), "BepInEx.Preloader.dll"), []byte("runtime"), 0644)
		bepCfg = filepath.Join(paths.ProfileConfigDir("template"), "BepInEx.cfg")
	}
	os.WriteFile(bepCfg, []byte("[Logging]\nEnabled = true\n"), 0644)

	if err := Create(paths, "withcfg"); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Check the config was copied
	copiedCfg := filepath.Join(paths.ProfileConfigDir("withcfg"), "BepInEx.cfg")
	data, err := os.ReadFile(copiedCfg)
	if err != nil {
		t.Fatalf("BepInEx.cfg not copied: %v", err)
	}
	if string(data) != "[Logging]\nEnabled = true\n" {
		t.Error("copied BepInEx.cfg content mismatch")
	}
}

func TestMigrateContentsFallsBackToCopyWhenRenameFails(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")
	if err := os.MkdirAll(filepath.Join(src, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "BepInEx.cfg"), []byte("config"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "nested", "mod.cfg"), []byte("mod"), 0644); err != nil {
		t.Fatal(err)
	}

	originalRename := renamePath
	renamePath = func(oldpath, newpath string) error {
		return os.ErrInvalid
	}
	t.Cleanup(func() { renamePath = originalRename })

	if err := migrateContents(src, dst); err != nil {
		t.Fatalf("migrateContents failed: %v", err)
	}

	for _, path := range []string{
		filepath.Join(dst, "BepInEx.cfg"),
		filepath.Join(dst, "nested", "mod.cfg"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing migrated file %s: %v", path, err)
		}
	}
	for _, path := range []string{
		filepath.Join(src, "BepInEx.cfg"),
		filepath.Join(src, "nested"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("source path should be removed after copy: %s", path)
		}
	}
}
