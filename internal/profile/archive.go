package profile

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"mmcli/internal/config"
)

// Archives contain installed files, not download links, so unavailable package
// versions and locally installed mods survive a move between computers.
type archiveManifest struct {
	Version  int                    `json:"format_version"`
	Name     string                 `json:"name"`
	Mods     []config.ModEntry      `json:"mods"`
	Settings config.ProfileSettings `json:"settings"`
}

func profileDirs(paths config.Paths, name string) map[string]string {
	return map[string]string{
		"plugins":  paths.ProfilePluginsDir(name),
		"config":   paths.ProfileConfigDir(name),
		"patchers": paths.ProfilePatchersDir(name),
		"monomod":  paths.ProfileMonomodDir(name),
	}
}

// ValidateName uses the same portable naming rules on every OS.
func ValidateName(name string) error {
	if err := portablePath(name); err != nil || strings.Contains(name, "/") {
		return fmt.Errorf("invalid profile name %q", name)
	}
	return nil
}

func portablePath(name string) error {
	if name == "" || path.Clean(name) != name || strings.HasPrefix(name, "/") {
		return fmt.Errorf("invalid archive path %q", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == "." || part == ".." || strings.TrimRight(part, " .") != part || strings.ContainsAny(part, "\\:<>\"|?*\x00") {
			return fmt.Errorf("non-portable path %q", name)
		}
		for _, c := range part {
			if c < 32 {
				return fmt.Errorf("non-portable path %q", name)
			}
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		runes := []rune(base)
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" ||
			(len(runes) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && strings.ContainsRune("123456789¹²³", runes[3])) {
			return fmt.Errorf("Windows reserved path %q", name)
		}
	}
	return nil
}

// ExportArchive never overwrites an existing archive or modifies the source.
// Global BepInEx core and loader files are deliberately not transferred.
func ExportArchive(paths config.Paths, reg config.Registry, name, destination string) (err error) {
	if err := ValidateName(name); err != nil {
		return err
	}
	if info, err := os.Stat(paths.ProfileDir(name)); err != nil || !info.IsDir() {
		return fmt.Errorf("profile %q does not exist", name)
	}
	dirs := profileDirs(paths, name)
	parent, err := filepath.EvalSymlinks(filepath.Dir(destination))
	if err != nil {
		return err
	}
	output, err := filepath.Abs(filepath.Join(parent, filepath.Base(destination)))
	if err != nil {
		return err
	}
	root, err := filepath.EvalSymlinks(paths.ProfileDir(name))
	if err != nil {
		return err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return err
	}
	if rel, e := filepath.Rel(root, output); e == nil && filepath.IsLocal(rel) {
		return fmt.Errorf("export destination must be outside the source profile")
	}
	manifest := archiveManifest{Version: 1, Name: name, Mods: reg.ListMods(name), Settings: reg.GetSettings(name)}
	// These settings point to machine-local resources. Connection credentials
	// live in config.json, which is never included in a profile archive.
	manifest.Settings.Server = ""
	manifest.Settings.ModpackPath = ""
	for i := range manifest.Mods {
		mod := &manifest.Mods[i]
		files := make([]string, 0, len(mod.Files))
		for _, file := range mod.Files {
			found := false
			for kind, dir := range dirs {
				rel, e := filepath.Rel(dir, file)
				if e == nil && filepath.IsLocal(rel) && rel != "." {
					files = append(files, kind+"/"+filepath.ToSlash(rel))
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("%s has a file outside portable profile directories: %s", mod.FullName(), file)
			}
		}
		mod.Files = files
	}
	f, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(destination)
		}
	}()
	zw := zip.NewWriter(f)
	w, err := zw.Create("mmcli-profile.json")
	if err != nil {
		return err
	}
	if err := json.NewEncoder(w).Encode(manifest); err != nil {
		return err
	}
	seen := make(map[string]bool)
	written := make(map[string]bool)
	for kind, dir := range dirs {
		if _, e := os.Stat(dir); os.IsNotExist(e) {
			continue
		}
		err = filepath.WalkDir(dir, func(filename string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, err := filepath.Rel(dir, filename)
			if err != nil {
				return err
			}
			entry := kind + "/" + filepath.ToSlash(rel)
			if rel == "." && d.IsDir() {
				return nil
			}
			if err := portablePath(entry); err != nil {
				return err
			}
			key := strings.ToLower(entry)
			if seen[key] {
				return fmt.Errorf("case-insensitive filename collision: %s", entry)
			}
			seen[key] = true
			if d.IsDir() {
				return nil
			}
			if !d.Type().IsRegular() {
				return fmt.Errorf("cannot transfer symlink or special file: %s", filename)
			}
			out, err := zw.Create(entry)
			if err != nil {
				return err
			}
			in, err := os.Open(filename)
			if err != nil {
				return err
			}
			defer in.Close()
			_, err = io.Copy(out, in)
			if err == nil {
				written[entry] = true
			}
			return err
		})
		if err != nil {
			return err
		}
	}
	for _, mod := range manifest.Mods {
		for _, file := range mod.Files {
			if !written[file] && !(mod.Disabled && strings.HasSuffix(strings.ToLower(file), ".dll") && written[file+".old"]) {
				return fmt.Errorf("tracked file missing from profile: %s", file)
			}
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return f.Close()
}

// ImportArchive restores into a new profile, rewrites registry paths for the
// destination OS, and keeps the active profile untouched. Failed imports roll
// back the new directory. Run init first to provide the native BepInEx runtime.
func ImportArchive(paths config.Paths, reg *config.Registry, name, source string) (err error) {
	if err := ValidateName(name); err != nil {
		return err
	}
	if _, exists := reg.Profiles[name]; exists {
		return fmt.Errorf("profile %q already exists in registry", name)
	}
	if _, e := os.Lstat(paths.ProfileDir(name)); !os.IsNotExist(e) {
		return fmt.Errorf("profile %q already exists or cannot be accessed", name)
	}
	zr, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer zr.Close()
	entries := make(map[string]*zip.File)
	seen := make(map[string]bool)
	pathCase := make(map[string]string)
	var manifest archiveManifest
	for _, entry := range zr.File {
		if err := portablePath(entry.Name); err != nil {
			return err
		}
		if !entry.Mode().IsRegular() {
			return fmt.Errorf("unsupported archive entry %q", entry.Name)
		}
		key := strings.ToLower(entry.Name)
		if seen[key] {
			return fmt.Errorf("duplicate archive entry %q", entry.Name)
		}
		seen[key] = true
		for p := entry.Name; p != "."; p = path.Dir(p) {
			key := strings.ToLower(p)
			if previous, exists := pathCase[key]; exists && previous != p {
				return fmt.Errorf("case-insensitive path collision: %s and %s", previous, p)
			}
			pathCase[key] = p
		}
		entries[entry.Name] = entry
	}
	mf, ok := entries["mmcli-profile.json"]
	if !ok {
		return fmt.Errorf("not an mmcli profile archive")
	}
	in, err := mf.Open()
	if err != nil {
		return err
	}
	err = json.NewDecoder(io.LimitReader(in, 8<<20)).Decode(&manifest)
	in.Close()
	if err != nil {
		return fmt.Errorf("invalid profile manifest: %w", err)
	}
	if manifest.Version != 1 {
		return fmt.Errorf("unsupported profile archive version %d", manifest.Version)
	}
	dirs := profileDirs(paths, name)
	destination := func(entry string) (string, error) {
		if err := portablePath(entry); err != nil {
			return "", err
		}
		parts := strings.SplitN(entry, "/", 2)
		if len(parts) != 2 || dirs[parts[0]] == "" {
			return "", fmt.Errorf("unsupported profile path %q", entry)
		}
		return filepath.Join(dirs[parts[0]], filepath.FromSlash(parts[1])), nil
	}
	for entry := range entries {
		if entry == "mmcli-profile.json" {
			continue
		}
		if _, err := destination(entry); err != nil {
			return err
		}
	}
	mods := make(map[string]config.ModEntry)
	for _, mod := range manifest.Mods {
		if err := ValidateName(mod.FullName()); err != nil {
			return err
		}
		if _, exists := mods[mod.FullName()]; exists {
			return fmt.Errorf("duplicate mod %q", mod.FullName())
		}
		for i, file := range mod.Files {
			dest, err := destination(file)
			if err != nil {
				return err
			}
			// Disabled DLLs retain their original registry path.
			if entries[file] == nil && !(mod.Disabled && strings.HasSuffix(strings.ToLower(file), ".dll") && entries[file+".old"] != nil) {
				return fmt.Errorf("tracked file missing from archive: %s", file)
			}
			mod.Files[i] = dest
		}
		mods[mod.FullName()] = mod
	}
	// Create and extract in a private staging root before publishing the profile.
	if err := os.MkdirAll(paths.ProfilesDir, 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(paths.ProfilesDir, ".mmcli-import-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	staged := paths
	staged.ProfilesDir = stage
	if err := Create(staged, name); err != nil {
		return err
	}
	// Windows runtime comes from an initialized destination profile, never
	// from the source computer's loader.
	if runtime.GOOS == "windows" {
		template, ok := findTemplateProfile(paths, name)
		if !ok {
			return fmt.Errorf("no native BepInEx runtime found; run mmcli init first")
		}
		if err := copyDirContents(paths.ProfileCoreDir(template), staged.ProfileCoreDir(name)); err != nil {
			return err
		}
	}
	stagedDirs := profileDirs(staged, name)
	for entry, zf := range entries {
		if entry == "mmcli-profile.json" {
			continue
		}
		parts := strings.SplitN(entry, "/", 2)
		dest := filepath.Join(stagedDirs[parts[0]], filepath.FromSlash(parts[1]))
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		if err := extractArchiveFile(zf, dest); err != nil {
			return err
		}
	}
	if err := os.Rename(staged.ProfileDir(name), paths.ProfileDir(name)); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(paths.ProfileDir(name))
		}
	}()
	updated := config.NewRegistry()
	for k, v := range reg.Profiles {
		updated.Profiles[k] = v
	}
	for k, v := range reg.Settings {
		updated.Settings[k] = v
	}
	updated.Profiles[name] = mods
	manifest.Settings.Server = ""
	manifest.Settings.ModpackPath = ""
	updated.Settings[name] = manifest.Settings
	if err := config.SaveRegistry(paths, updated); err != nil {
		return err
	}
	*reg = updated
	return nil
}

func extractArchiveFile(zf *zip.File, destination string) error {
	in, err := zf.Open()
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
