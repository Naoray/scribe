package add

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Naoray/scribe/internal/discovery"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/migrate"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)


// Candidate represents a skill that can be added to a registry.
type Candidate struct {
	Name        string // skill name (directory basename)
	Description string // short description from SKILL.md
	Origin      string // "local" or "registry:owner/repo"
	Package     string // parent package name if sub-skill (e.g. "gstack")
	Source      string // "github:owner/repo@ref" or empty for local-only
	LocalPath   string // absolute path on disk, empty for remote-only
}

// NeedsUpload reports whether this candidate requires uploading files to the
// registry (as opposed to just adding a source reference to scribe.yaml).
func (c Candidate) NeedsUpload() bool {
	return c.Source == "" && c.LocalPath != ""
}

// Adder wires discovery and GitHub push together.
// Emits events via the Emit callback — the caller decides output format.
type Adder struct {
	Client  *gh.Client
	Tools []tools.Tool
	Emit    func(any)
}

func (a *Adder) emit(msg any) {
	if a.Emit != nil {
		a.Emit(msg)
	}
}

// DiscoverLocal scans ~/.claude/skills/ and ~/.scribe/skills/ for skills on disk.
// Cross-references state for source info. Deduplicates by name (first seen wins).
func (a *Adder) DiscoverLocal(st *state.State) ([]Candidate, error) {
	skills, err := discovery.OnDisk(st)
	if err != nil {
		return nil, err
	}

	candidates := make([]Candidate, 0, len(skills))
	for _, sk := range skills {
		candidates = append(candidates, Candidate{
			Name:        sk.Name,
			Description: sk.Description,
			Origin:      "local",
			Package:     sk.Package,
			LocalPath:   sk.LocalPath,
		})
	}

	return candidates, nil
}

// DiscoverRemote finds skills in other registries that are not in the target registry.
// Takes pre-fetched manifests to keep GitHub calls in the cmd layer.
func (a *Adder) DiscoverRemote(targetManifest *manifest.Manifest, otherManifests map[string]*manifest.Manifest) []Candidate {
	var candidates []Candidate

	targetNames := make(map[string]bool, len(targetManifest.Catalog))
	for _, e := range targetManifest.Catalog {
		targetNames[e.Name] = true
	}

	for registry, m := range otherManifests {
		for _, entry := range m.Catalog {
			// Skip if already in target registry.
			if targetNames[entry.Name] {
				continue
			}

			candidates = append(candidates, Candidate{
				Name:   entry.Name,
				Origin: "registry:" + registry,
				Source: entry.Source,
			})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Name < candidates[j].Name
	})

	return candidates
}

// maxFileSize is the per-file size limit when uploading local skills (1 MB).
const maxFileSize = 1 << 20

// ReadLocalSkillFiles reads all files from a local skill directory and returns
// them as a map of "skills/<name>/<relative-path>" → content string.
// Used when uploading a local-only skill to a registry.
// Rejects symlinks and files larger than maxFileSize.
func ReadLocalSkillFiles(c Candidate) (map[string]string, error) {
	files := map[string]string{}
	root := c.LocalPath

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Reject symlinks to prevent traversal outside the skill directory.
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", rel, err)
		}
		if info.Size() > maxFileSize {
			return fmt.Errorf("%s exceeds %d byte limit (%d bytes)", rel, maxFileSize, info.Size())
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", rel, err)
		}
		key := "skills/" + c.Name + "/" + filepath.ToSlash(rel)
		files[key] = string(content)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}
	return files, nil
}

// Add pushes one or more skills to the target registry's scribe.yaml on GitHub
// in a single atomic commit. For each candidate: adds a source reference or
// uploads files + self-reference. Emits events throughout.
func (a *Adder) Add(ctx context.Context, targetRepo string, candidates []Candidate) error {
	owner, repo, err := manifest.ParseOwnerRepo(targetRepo)
	if err != nil {
		return fmt.Errorf("parse target registry %q: %w", targetRepo, err)
	}

	// Fetch the current manifest with fallback.
	m, err := a.fetchManifest(ctx, owner, repo)
	if err != nil {
		return fmt.Errorf("fetch manifest: %w", err)
	}

	// Accumulate all files to push in one commit.
	pushFiles := map[string]string{}
	var added []string
	var failed int

	for _, c := range candidates {
		// Skip if already in catalog (prevents duplicates within a batch).
		if m.FindByName(c.Name) != nil {
			continue
		}

		a.emit(SkillAddingMsg{Name: c.Name, Upload: c.NeedsUpload()})

		if c.NeedsUpload() {
			localFiles, err := ReadLocalSkillFiles(c)
			if err != nil {
				a.emit(SkillAddErrorMsg{Name: c.Name, Err: err})
				failed++
				continue
			}
			for k, v := range localFiles {
				pushFiles[k] = v
			}
			m.Catalog = append(m.Catalog, manifest.Entry{
				Name:   c.Name,
				Source: fmt.Sprintf("github:%s/%s@main", owner, repo),
				Path:   "skills/" + c.Name,
			})
		} else {
			m.Catalog = append(m.Catalog, manifest.Entry{
				Name:   c.Name,
				Source: c.Source,
			})
		}

		source := c.Source
		if c.NeedsUpload() {
			source = fmt.Sprintf("github:%s/%s@main", owner, repo)
		}
		a.emit(SkillAddedMsg{
			Name:     c.Name,
			Registry: targetRepo,
			Source:   source,
			Upload:   c.NeedsUpload(),
		})
		added = append(added, c.Name)
	}

	if len(added) == 0 {
		return fmt.Errorf("all %d skills failed to add", failed)
	}

	encoded, err := m.Encode()
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	pushFiles[manifest.ManifestFilename] = string(encoded)

	msg := fmt.Sprintf("add skills: %s", strings.Join(added, ", "))
	return a.Client.PushFiles(ctx, owner, repo, pushFiles, msg)
}

// AddPackageRef fetches a package manifest from packageRepo and adds a
// package catalog entry to the target registry's scribe.yaml. The package's
// declared per-tool install commands (package.installs) are copied into the
// catalog entry; the global install field is used as a fallback if no per-tool
// commands are declared.
func (a *Adder) AddPackageRef(ctx context.Context, targetRepo, packageRepo string) error {
	targetOwner, targetRepoName, err := manifest.ParseOwnerRepo(targetRepo)
	if err != nil {
		return fmt.Errorf("parse target registry %q: %w", targetRepo, err)
	}

	pkgOwner, pkgRepoName, err := manifest.ParseOwnerRepo(packageRepo)
	if err != nil {
		return fmt.Errorf("parse package repo %q: %w", packageRepo, err)
	}

	// Fetch the package's own manifest to read metadata + install scripts.
	pkgManifest, err := a.fetchManifest(ctx, pkgOwner, pkgRepoName)
	if err != nil {
		return fmt.Errorf("fetch package manifest for %s: %w", packageRepo, err)
	}
	if !pkgManifest.IsPackage() {
		return fmt.Errorf("%s is not a package (kind: %s)", packageRepo, pkgManifest.Kind)
	}

	pkg := pkgManifest.Package
	if len(pkg.Installs) == 0 && pkg.Updates == nil {
		return fmt.Errorf("%s declares no install commands — add installs: to its package.installs section", packageRepo)
	}

	// Determine the author from the repo owner if not set in authors list.
	author := pkgOwner
	if len(pkg.Authors) > 0 {
		author = pkg.Authors[0]
	}

	entry := manifest.Entry{
		Name:     pkg.Name,
		Source:   fmt.Sprintf("github:%s/%s@main", pkgOwner, pkgRepoName),
		Type:     manifest.EntryTypePackage,
		Installs: pkg.Installs,
		Updates:  pkg.Updates,
		Author:   author,
	}

	a.emit(SkillAddingMsg{Name: pkg.Name, Upload: false})

	// Fetch and update the target registry manifest.
	m, err := a.fetchManifest(ctx, targetOwner, targetRepoName)
	if err != nil {
		return fmt.Errorf("fetch manifest: %w", err)
	}
	if m.FindByName(pkg.Name) != nil {
		return fmt.Errorf("%q is already in %s", pkg.Name, targetRepo)
	}

	m.Catalog = append(m.Catalog, entry)
	encoded, err := m.Encode()
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}

	msg := fmt.Sprintf("add package: %s", pkg.Name)
	if err := a.Client.PushFiles(ctx, targetOwner, targetRepoName, map[string]string{
		manifest.ManifestFilename: string(encoded),
	}, msg); err != nil {
		return err
	}

	a.emit(SkillAddedMsg{
		Name:     pkg.Name,
		Registry: targetRepo,
		Source:   entry.Source,
		Upload:   false,
	})
	return nil
}

// fetchManifest tries scribe.yaml first, falls back to scribe.toml (converting via migrate).
func (a *Adder) fetchManifest(ctx context.Context, owner, repo string) (*manifest.Manifest, error) {
	m, _, err := manifest.FetchWithFallback(ctx, a.Client, owner, repo, migrate.Convert)
	return m, err
}

