package installer

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"mmcli/internal/config"
)

func TestExtractModOverrideFolders(t *testing.T) {
	for _, prefix := range []string{"", "BepInEx/", "bEpInEx/"} {
		t.Run("prefix="+prefix, func(t *testing.T) {
			tmp := t.TempDir()
			paths := config.Paths{ProfilesDir: filepath.Join(tmp, "profiles"), ValheimDir: filepath.Join(tmp, "game")}
			cfg := config.Config{ActiveProfile: "test"}
			mod := "Azumatt-Minimal_UI"
			entries := []struct {
				name string
				dest string
			}{
				{prefix + "plugins/MinimalUI.dll", filepath.Join(paths.ProfilePluginsDir("test"), mod, "MinimalUI.dll")},
				{prefix + "config/Azumatt.MinimalUI_Backgrounds/Auga.zip", filepath.Join(paths.ProfileConfigDir("test"), "Azumatt.MinimalUI_Backgrounds", "Auga.zip")},
				{prefix + "config/Azumatt.MinimalUI_Backgrounds/Auga/MUI_PlayerInvPanel295x154.png", filepath.Join(paths.ProfileConfigDir("test"), "Azumatt.MinimalUI_Backgrounds", "Auga", "MUI_PlayerInvPanel295x154.png")},
				{prefix + "patchers/patch.dll", filepath.Join(paths.ProfilePatchersDir("test"), mod, "patch.dll")},
				{prefix + "monomod/patch.mm.dll", filepath.Join(paths.ProfileMonomodDir("test"), mod, "patch.mm.dll")},
				{prefix + "core/library.dll", filepath.Join(paths.ProfileCoreDir("test"), mod, "library.dll")},
				{"assets/data.txt", filepath.Join(paths.ProfilePluginsDir("test"), mod, "assets", "data.txt")},
				{"root.dll", filepath.Join(paths.ProfilePluginsDir("test"), mod, "root.dll")},
				{"root.mm.dll", filepath.Join(paths.ProfileMonomodDir("test"), mod, "root.mm.dll")},
				{"README.md", ""},
				{"manifest.json", ""},
				{"icon.png", ""},
			}
			zipPath := filepath.Join(tmp, "mod.zip")
			f, err := os.Create(zipPath)
			if err != nil {
				t.Fatal(err)
			}
			zw := zip.NewWriter(f)
			for _, entry := range entries {
				w, err := zw.Create(entry.name)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := w.Write([]byte(entry.name)); err != nil {
					t.Fatal(err)
				}
			}
			if err := zw.Close(); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			files, err := extractMod(paths, cfg, "Azumatt", "Minimal_UI", zipPath)
			if err != nil {
				t.Fatal(err)
			}
			tracked := make(map[string]bool)
			for _, file := range files {
				tracked[file] = true
			}
			wantCount := 0
			for _, entry := range entries {
				if entry.dest == "" {
					continue
				}
				wantCount++
				data, err := os.ReadFile(entry.dest)
				if err != nil {
					t.Errorf("%s: %v", entry.name, err)
				} else if string(data) != entry.name {
					t.Errorf("%s: content changed during extraction", entry.name)
				}
				if !tracked[entry.dest] {
					t.Errorf("%s: destination missing from tracked files", entry.name)
				}
			}
			if len(files) != wantCount {
				t.Errorf("tracked %d files, want %d (metadata must be skipped)", len(files), wantCount)
			}
		})
	}
}

func TestRemoveModFilesOnlyRemovesProfileConfigs(t *testing.T) {
	root := t.TempDir()
	paths := config.Paths{ProfilesDir: filepath.Join(root, "profiles"), ValheimDir: filepath.Join(root, "game")}
	inside := filepath.Join(paths.ProfileConfigDir("test"), "nested", "mod.cfg")
	outside := filepath.Join(root, "other", "config", "keep.cfg")
	for _, file := range []string{inside, outside} {
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("settings"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	removeModFiles(paths, config.Config{ActiveProfile: "test"}, config.ModEntry{Owner: "Author", Name: "Mod", Files: []string{inside, outside}})
	if _, err := os.Stat(inside); !os.IsNotExist(err) {
		t.Fatal("profile config was not removed")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("removed another profile's config")
	}
}

func TestFindMod(t *testing.T) {
	reg := config.NewRegistry()
	reg.SetMod("default", config.ModEntry{Owner: "RandyKnapp", Name: "EpicLoot", Version: "1.0.0"})
	reg.SetMod("default", config.ModEntry{Owner: "Smoothbrain", Name: "Jewelcrafting", Version: "2.0.0"})

	tests := []struct {
		name      string
		query     string
		wantFound bool
		wantName  string
	}{
		{"exact full name", "RandyKnapp-EpicLoot", true, "EpicLoot"},
		{"just mod name", "EpicLoot", true, "EpicLoot"},
		{"case insensitive name", "epicloot", true, "EpicLoot"},
		{"case insensitive full name", "randyknapp-epicloot", true, "EpicLoot"},
		{"not found", "NonExistent", false, ""},
		{"partial match should not work", "Epic", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mod, found := findMod(&reg, "default", tt.query)
			if found != tt.wantFound {
				t.Errorf("findMod(%q) found = %v, want %v", tt.query, found, tt.wantFound)
			}
			if found && mod.Name != tt.wantName {
				t.Errorf("findMod(%q) name = %q, want %q", tt.query, mod.Name, tt.wantName)
			}
		})
	}
}

func TestFindModEmptyProfile(t *testing.T) {
	reg := config.NewRegistry()
	_, found := findMod(&reg, "empty", "anything")
	if found {
		t.Error("findMod should return false for empty profile")
	}
}

func TestIsLocalPath(t *testing.T) {
	// Create a temp file and directory to test with
	tmp := t.TempDir()
	tmpFile := filepath.Join(tmp, "test.dll")
	os.WriteFile(tmpFile, []byte("data"), 0644)

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"existing directory", tmp, true},
		{"existing file", tmpFile, true},
		{"non-existent path", "/nonexistent/path/to/file", false},
		{"thunderstore query", "RandyKnapp-EpicLoot", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsLocalPath(tt.input)
			if got != tt.expected {
				t.Errorf("IsLocalPath(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"tilde only", "~", home},
		{"tilde with path", "~/Documents", filepath.Join(home, "Documents")},
		{"no tilde", "/absolute/path", "/absolute/path"},
		{"relative", "relative/path", "relative/path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandHome(tt.input)
			if got != tt.expected {
				t.Errorf("expandHome(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestDetectLocalModsIntegration(t *testing.T) {
	tmp := t.TempDir()

	// Set up a plugins directory structure
	os.MkdirAll(filepath.Join(tmp, "TrackedMod"), 0755)
	os.WriteFile(filepath.Join(tmp, "TrackedMod", "mod.dll"), []byte("dll"), 0644)
	os.MkdirAll(filepath.Join(tmp, "UnknownMod"), 0755)
	os.WriteFile(filepath.Join(tmp, "UnknownMod", "plugin.dll"), []byte("dll"), 0644)

	registered := map[string]config.ModEntry{
		"TrackedMod": {Name: "TrackedMod", IsLocal: true},
	}

	locals := config.DetectLocalMods(tmp, registered)

	if len(locals) != 1 {
		t.Fatalf("got %d local mods, want 1", len(locals))
	}
	if locals[0].Name != "UnknownMod" {
		t.Errorf("local mod name = %q, want %q", locals[0].Name, "UnknownMod")
	}
	if !locals[0].IsLocal {
		t.Error("detected mod should be marked as local")
	}
}
