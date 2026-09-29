package profile

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"mmcli/internal/config"
)

func archivePaths(t *testing.T) config.Paths {
	t.Helper()
	p := testPaths(t)
	p.RegistryFile = filepath.Join(p.ConfigDir, "registry.json")
	if err := Create(p, "default"); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(p.ProfileCoreDir("default"), "BepInEx.Preloader.dll"), "native-runtime")
	return p
}

func writeFixture(t *testing.T, filename, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestArchiveRoundTrip(t *testing.T) {
	source, destination := archivePaths(t), archivePaths(t)
	reg := config.NewRegistry()
	plugin := filepath.Join(source.ProfilePluginsDir("default"), "Author-Mod", "Mod.dll")
	configFile := filepath.Join(source.ProfileConfigDir("default"), "nested", "settings.cfg")
	writeFixture(t, plugin+".old", "exact-disabled-binary")
	writeFixture(t, configFile, "user settings")
	writeFixture(t, filepath.Join(source.ProfilePluginsDir("default"), "Local.dll"), "unregistered local mod")
	writeFixture(t, filepath.Join(source.ProfilePatchersDir("default"), "Patcher.dll"), "patcher")
	writeFixture(t, filepath.Join(source.ProfileMonomodDir("default"), "Patch.mm.dll"), "monomod")
	mod := config.ModEntry{Owner: "Author", Name: "Mod", Version: "1.2.3", Source: "hexium", Disabled: true, IsDependency: true, Dependencies: []string{"Author-Dependency"}, Files: []string{plugin, configFile}}
	reg.SetMod("default", mod)
	reg.SetSettings("default", config.ProfileSettings{Server: "private-connection", ModpackPath: "/old/machine/path", AnticheatSystem: "azu"})
	archive := filepath.Join(t.TempDir(), "profile.zip")
	if err := ExportArchive(source, reg, "default", archive); err != nil {
		t.Fatal(err)
	}
	if reg.ListMods("default")[0].Files[0] != plugin {
		t.Fatal("export modified original registry")
	}
	if err := ExportArchive(source, reg, "default", archive); err == nil {
		t.Fatal("overwrote archive")
	}
	restored := config.NewRegistry()
	restored.SetMod("default", config.ModEntry{Owner: "Existing", Name: "Mod"})
	if err := ImportArchive(destination, &restored, "moved", archive); err != nil {
		t.Fatal(err)
	}
	got, ok := restored.GetMod("moved", "Author-Mod")
	if !ok {
		t.Fatal("missing restored mod")
	}
	mod.Files = []string{filepath.Join(destination.ProfilePluginsDir("moved"), "Author-Mod", "Mod.dll"), filepath.Join(destination.ProfileConfigDir("moved"), "nested", "settings.cfg")}
	if !reflect.DeepEqual(got, mod) {
		t.Fatalf("metadata mismatch: got %+v want %+v", got, mod)
	}
	for file, want := range map[string]string{
		mod.Files[0] + ".old": "exact-disabled-binary", mod.Files[1]: "user settings",
		filepath.Join(destination.ProfilePluginsDir("moved"), "Local.dll"):    "unregistered local mod",
		filepath.Join(destination.ProfilePatchersDir("moved"), "Patcher.dll"): "patcher",
		filepath.Join(destination.ProfileMonomodDir("moved"), "Patch.mm.dll"): "monomod",
	} {
		data, err := os.ReadFile(file)
		if err != nil || string(data) != want {
			t.Fatalf("%s did not survive: %v", file, err)
		}
	}
	ps := restored.GetSettings("moved")
	if ps.Server != "" || ps.ModpackPath != "" || ps.AnticheatSystem != "azu" {
		t.Fatalf("unexpected settings: %+v", ps)
	}
	if _, ok := restored.GetMod("default", "Existing-Mod"); !ok {
		t.Fatal("changed existing profile")
	}
	if err := ImportArchive(destination, &restored, "moved", archive); err == nil {
		t.Fatal("overwrote profile")
	}
	disk, err := config.LoadRegistry(destination)
	if err != nil || !reflect.DeepEqual(disk, restored) {
		t.Fatal("registry was not persisted")
	}
}

func makeArchive(t *testing.T, entries map[string]string) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "profile.zip")
	f, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return filename
}

func TestArchiveRejectsUnsafePathsAndIncompleteMods(t *testing.T) {
	for _, unsafe := range []string{"../escape", "/absolute", "plugins/../../escape", `plugins\escape.dll`, "plugins/C:escape", "plugins/NUL.dll", "plugins/trailing.", "core/foreign-runtime.dll"} {
		t.Run(unsafe, func(t *testing.T) {
			p := archivePaths(t)
			archive := makeArchive(t, map[string]string{"mmcli-profile.json": `{"format_version":1}`, unsafe: "bad"})
			reg := config.NewRegistry()
			if err := ImportArchive(p, &reg, "imported", archive); err == nil {
				t.Fatal("accepted unsafe archive")
			}
			if _, err := os.Stat(p.ProfileDir("imported")); !os.IsNotExist(err) {
				t.Fatal("left partial profile")
			}
		})
	}
	p := archivePaths(t)
	m := archiveManifest{Version: 1, Mods: []config.ModEntry{{Owner: "Author", Name: "Mod", Files: []string{"plugins/missing.dll"}}}}
	data, _ := json.Marshal(m)
	reg := config.NewRegistry()
	if err := ImportArchive(p, &reg, "incomplete", makeArchive(t, map[string]string{"mmcli-profile.json": string(data)})); err == nil {
		t.Fatal("accepted missing mod binary")
	}
}

func TestImportArchiveRollsBackOnRegistryFailure(t *testing.T) {
	p := archivePaths(t)
	p.RegistryFile = filepath.Join(p.ConfigDir, "missing", "registry.json")
	reg := config.NewRegistry()
	archive := makeArchive(t, map[string]string{"mmcli-profile.json": `{"format_version":1}`, "plugins/Local.dll": "binary"})
	if err := ImportArchive(p, &reg, "rollback", archive); err == nil {
		t.Fatal("expected registry failure")
	}
	if _, err := os.Stat(p.ProfileDir("rollback")); !os.IsNotExist(err) {
		t.Fatal("left partial profile")
	}
	if len(reg.Profiles) != 0 {
		t.Fatal("mutated registry on failure")
	}
}

func TestPortableProfileNames(t *testing.T) {
	for _, name := range []string{"", ".", "..", "../other", "a/b", `a\b`, "CON", "nul.txt", "COM1", "COM¹", "LPT9.txt", "trailing.", "trailing ", "a:b"} {
		if err := ValidateName(name); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
	for _, name := range []string{"default", "my-server", "My Mods", "日本語"} {
		if err := ValidateName(name); err != nil {
			t.Error(err)
		}
	}
}

func TestExportRejectsDestinationInsideProfile(t *testing.T) {
	p := archivePaths(t)
	dest := filepath.Join(p.ProfilePluginsDir("default"), "recursive.zip")
	if err := ExportArchive(p, config.NewRegistry(), "default", dest); err == nil {
		t.Fatal("accepted recursive export")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("left partial archive")
	}
}

func TestExportRejectsMissingTrackedFiles(t *testing.T) {
	p := archivePaths(t)
	reg := config.NewRegistry()
	reg.SetMod("default", config.ModEntry{Owner: "Author", Name: "Mod", Files: []string{filepath.Join(p.ProfilePluginsDir("default"), "missing.dll")}})
	dest := filepath.Join(t.TempDir(), "missing.zip")
	if err := ExportArchive(p, reg, "default", dest); err == nil {
		t.Fatal("exported incomplete profile")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("left partial archive")
	}
}

func TestImportRejectsCaseCollisionsInParentDirectories(t *testing.T) {
	p := archivePaths(t)
	archive := makeArchive(t, map[string]string{"mmcli-profile.json": `{"format_version":1}`, "plugins/Mod/a.dll": "a", "plugins/mod/b.dll": "b"})
	reg := config.NewRegistry()
	if err := ImportArchive(p, &reg, "collision", archive); err == nil {
		t.Fatal("accepted parent directory collision")
	}
}

func TestArchiveRejectsCaseCollisions(t *testing.T) {
	p := archivePaths(t)
	archive := makeArchive(t, map[string]string{"mmcli-profile.json": `{"format_version":1}`, "plugins/Mod.dll": "a", "plugins/mod.dll": "b"})
	reg := config.NewRegistry()
	if err := ImportArchive(p, &reg, "collision", archive); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected collision error, got %v", err)
	}
}
