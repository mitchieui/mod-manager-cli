package thunderstore

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const (
	baseURL            = "https://thunderstore.io"
	experimentalAPI    = baseURL + "/api/experimental/package/"
	SourceThunderstore = "thunderstore"
	SourceHexium       = "hexium"
)

var sourceAPIs = map[string]string{
	SourceThunderstore: experimentalAPI,
	SourceHexium:       "https://hexium.gg/api/experimental/package/",
}

// GetPackageVersion fetches a specific version of a package from the experimental API.
func GetPackageVersion(owner, name, version string) (*Package, error) {
	return GetPackageVersionFrom(SourceThunderstore, owner, name, version)
}

// GetPackageVersionFrom fetches a specific package version from a supported source.
func GetPackageVersionFrom(source, owner, name, version string) (*Package, error) {
	if source == "" {
		source = SourceThunderstore
	}
	api, err := apiForSource(source)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("%s%s/%s/%s/", api, url.PathEscape(owner), url.PathEscape(name), url.PathEscape(version))
	resp, err := http.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch package version: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("version %s of %s-%s not found (HTTP %d)", version, owner, name, resp.StatusCode)
	}

	var ev ExperimentalVersion
	if err := json.NewDecoder(resp.Body).Decode(&ev); err != nil {
		return nil, fmt.Errorf("failed to decode version response: %w", err)
	}

	pkg := &Package{
		Source:   source,
		Owner:    owner,
		Name:     name,
		FullName: fmt.Sprintf("%s-%s", owner, name),
		Versions: []Version{
			{
				Name:          name,
				FullName:      fmt.Sprintf("%s-%s-%s", owner, name, ev.VersionNumber),
				VersionNumber: ev.VersionNumber,
				DownloadURL:   ev.DownloadURL,
				Dependencies:  ev.Dependencies,
				Description:   ev.Description,
				FileSize:      ev.FileSize,
			},
		},
	}
	return pkg, nil
}

// GetPackage fetches a single package from the experimental API.
func GetPackage(owner, name string) (*Package, error) {
	return GetPackageFrom(SourceThunderstore, owner, name)
}

// GetPackageFrom fetches the latest package version from a supported source.
func GetPackageFrom(source, owner, name string) (*Package, error) {
	if source == "" {
		source = SourceThunderstore
	}
	api, err := apiForSource(source)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("%s%s/%s/", api, url.PathEscape(owner), url.PathEscape(name))
	resp, err := http.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch package: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("package %s-%s not found (HTTP %d)", owner, name, resp.StatusCode)
	}

	var expPkg ExperimentalPackage
	if err := json.NewDecoder(resp.Body).Decode(&expPkg); err != nil {
		return nil, fmt.Errorf("failed to decode package response: %w", err)
	}

	// Convert to Package type
	pkg := &Package{
		Source:   source,
		Owner:    expPkg.Namespace,
		Name:     expPkg.Name,
		FullName: expPkg.FullName,
		Versions: []Version{
			{
				Name:          expPkg.Name,
				FullName:      fmt.Sprintf("%s-%s-%s", expPkg.Namespace, expPkg.Name, expPkg.LatestVersion.VersionNumber),
				VersionNumber: expPkg.LatestVersion.VersionNumber,
				DownloadURL:   expPkg.LatestVersion.DownloadURL,
				Dependencies:  expPkg.LatestVersion.Dependencies,
				Description:   expPkg.LatestVersion.Description,
				FileSize:      expPkg.LatestVersion.FileSize,
			},
		},
	}
	return pkg, nil
}

func apiForSource(source string) (string, error) {
	if source == "" {
		source = SourceThunderstore
	}
	api, ok := sourceAPIs[source]
	if !ok {
		return "", fmt.Errorf("unsupported mod source %q", source)
	}
	return api, nil
}

// ResolveDependencies resolves all dependencies for a package recursively.
// Returns packages in topological order (dependencies first).
// Skips BepInExPack_Valheim and already-installed mods.
func ResolveDependencies(pkg *Package, installed map[string]bool) ([]DepRef, error) {
	if len(pkg.Versions) == 0 {
		return nil, fmt.Errorf("package %s has no versions", pkg.FullName)
	}

	var result []DepRef
	visited := make(map[string]bool)
	inStack := make(map[string]bool)

	var dfs func(source string, deps []string) error
	dfs = func(source string, deps []string) error {
		for _, dep := range deps {
			ref := ParseDep(dep)
			fullName := fmt.Sprintf("%s-%s", ref.Owner, ref.Name)

			// Skip BepInExPack
			if ref.Name == "BepInExPack_Valheim" || ref.Name == "BepInEx_pack" {
				continue
			}

			// Skip already installed
			if installed[fullName] {
				continue
			}

			// Skip already visited
			if visited[fullName] {
				continue
			}

			// Cycle detection
			if inStack[fullName] {
				continue // Skip cycles silently
			}

			inStack[fullName] = true

			// Fetch the pinned dependency version so its dependency graph also
			// matches the version required by the package manifest.
			var depPkg *Package
			var err error
			depPkg, ref.Source, err = getDependency(source, ref)
			if err != nil {
				// Non-fatal: some deps may not resolve
				fmt.Printf("  Warning: could not resolve dependency %s: %v\n", fullName, err)
				inStack[fullName] = false
				continue
			}

			if len(depPkg.Versions) > 0 {
				if err := dfs(depPkg.Source, depPkg.Versions[0].Dependencies); err != nil {
					return err
				}
			}

			inStack[fullName] = false
			visited[fullName] = true
			result = append(result, DepRef{
				Owner:   ref.Owner,
				Name:    ref.Name,
				Version: ref.Version,
				Source:  ref.Source,
			})
		}
		return nil
	}

	if err := dfs(pkg.Source, pkg.Versions[0].Dependencies); err != nil {
		return nil, err
	}

	return result, nil
}

// FindPackageByQuery searches for a package by query string.
// Accepts "Owner-Name", "Owner-Name-Version", a supported mod URL, or
// "hexium:Owner-Name" / "thunderstore:Owner-Name" for explicit selection.
func FindPackageByQuery(query string) (*Package, error) {
	source := ""
	for _, candidate := range []string{SourceHexium, SourceThunderstore} {
		prefix := candidate + ":"
		if strings.HasPrefix(strings.ToLower(query), prefix) {
			source = candidate
			query = query[len(prefix):]
			break
		}
	}

	// Parse Thunderstore and Hexium package URLs.
	if parsed, err := url.Parse(query); err == nil && parsed.Host != "" {
		host := strings.ToLower(parsed.Hostname())
		switch {
		case host == "thunderstore.io" || strings.HasSuffix(host, ".thunderstore.io"):
			source = SourceThunderstore
		case host == "hexium.gg" || strings.HasSuffix(host, ".hexium.gg"):
			source = SourceHexium
		default:
			return nil, fmt.Errorf("unsupported mod URL: %s", query)
		}
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		// Thunderstore uses /p/Owner/Name; Hexium uses /mods/Owner/Name.
		for i, p := range parts {
			if (p == "p" || p == "mods") && i+2 < len(parts) {
				pkg, err := GetPackageFrom(source, parts[i+1], parts[i+2])
				if err == nil {
					return pkg, nil
				}
				return nil, fmt.Errorf("could not fetch package from URL: %w", err)
			}
		}
		return nil, fmt.Errorf("could not parse mod URL: %s", query)
	}

	sources := []string{source}
	if source == "" {
		sources = []string{SourceThunderstore, SourceHexium}
	}

	// Try parsing as Owner-Name-Version first (e.g., "warpalicious-Praetoris-1.1.16")
	ref := ParseDep(query)
	if ref.Owner != "" && ref.Name != "" {
		for _, candidate := range sources {
			pkg, err := GetPackageFrom(candidate, ref.Owner, ref.Name)
			if err == nil {
				return pkg, nil
			}
		}
	}

	// Try as Owner-Name (e.g., "warpalicious-Praetoris")
	parts := strings.SplitN(query, "-", 2)
	if len(parts) == 2 {
		for _, candidate := range sources {
			pkg, err := GetPackageFrom(candidate, parts[0], parts[1])
			if err == nil {
				return pkg, nil
			}
		}
	}

	return nil, fmt.Errorf("no package found matching '%s' — use Owner-Name, hexium:Owner-Name, or a package URL", query)
}

func getDependency(preferredSource string, ref DepRef) (*Package, string, error) {
	sources := []string{preferredSource}
	if preferredSource == "" {
		sources[0] = SourceThunderstore
	}
	if sources[0] == SourceThunderstore {
		sources = append(sources, SourceHexium)
	} else {
		sources = append(sources, SourceThunderstore)
	}
	var lastErr error
	for _, source := range sources {
		var pkg *Package
		if ref.Version == "" {
			pkg, lastErr = GetPackageFrom(source, ref.Owner, ref.Name)
		} else {
			pkg, lastErr = GetPackageVersionFrom(source, ref.Owner, ref.Name, ref.Version)
		}
		if lastErr == nil {
			return pkg, source, nil
		}
	}
	return nil, "", lastErr
}
