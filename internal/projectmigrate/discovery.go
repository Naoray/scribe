package projectmigrate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Naoray/scribe/internal/projectfile"
	"github.com/Naoray/scribe/internal/state"
)

// GlobalSymlink is a Scribe-managed skill link in a tool's legacy global
// skills directory.
type GlobalSymlink struct {
	Tool          string `json:"tool"`
	Skill         string `json:"skill"`
	Path          string `json:"path"`
	CanonicalPath string `json:"canonical_path"`
}

// ProjectCandidate is a directory that can receive a project-local .scribe.yaml.
type ProjectCandidate struct {
	Path   string `json:"path"`
	Source string `json:"source"`
}

// DiscoveryOptions configures project migration discovery.
type DiscoveryOptions struct {
	HomeDir     string
	StoreDir    string
	ToolNames   []string
	SearchRoots []string
	State       *state.State
}

// Discovery is the pure discovery result used by both interactive and JSON modes.
type Discovery struct {
	GlobalSymlinks []GlobalSymlink    `json:"global_symlinks"`
	Projects       []ProjectCandidate `json:"projects"`
	Skills         []string           `json:"skills"`
}

// DefaultFilesystemToolNames are tools with predictable ~/.<tool>/skills
// directories. Gemini is intentionally absent because its CLI owns storage.
func DefaultFilesystemToolNames() []string {
	return []string{"claude", "cursor", "codex"}
}

// Discover finds legacy global symlinks and candidate project directories.
func Discover(opts DiscoveryOptions) (Discovery, error) {
	toolNames := opts.ToolNames
	if len(toolNames) == 0 {
		toolNames = DefaultFilesystemToolNames()
	}

	links, err := DiscoverGlobalSymlinks(opts.HomeDir, opts.StoreDir, toolNames)
	if err != nil {
		return Discovery{}, err
	}

	projects, err := DiscoverCandidateProjects(opts.SearchRoots, opts.State)
	if err != nil {
		return Discovery{}, err
	}

	return Discovery{
		GlobalSymlinks: links,
		Projects:       projects,
		Skills:         uniqueSkills(links),
	}, nil
}

// DiscoverGlobalSymlinks scans ~/.<tool>/skills for symlinks pointing into the
// canonical Scribe skill store.
func DiscoverGlobalSymlinks(homeDir, storeDir string, toolNames []string) ([]GlobalSymlink, error) {
	if homeDir == "" {
		return nil, errors.New("home dir is required")
	}
	if storeDir == "" {
		return nil, errors.New("store dir is required")
	}

	storeDir, err := filepath.Abs(storeDir)
	if err != nil {
		return nil, fmt.Errorf("resolve store dir: %w", err)
	}

	var links []GlobalSymlink
	for _, tool := range toolNames {
		if tool == "" {
			continue
		}
		skillsDir := filepath.Join(homeDir, "."+tool, "skills")
		entries, err := os.ReadDir(skillsDir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s skills dir: %w", tool, err)
		}

		for _, entry := range entries {
			linkPath := filepath.Join(skillsDir, entry.Name())
			info, err := os.Lstat(linkPath)
			if err != nil {
				return nil, fmt.Errorf("stat global skill link: %w", err)
			}
			if info.Mode()&os.ModeSymlink == 0 {
				continue
			}
			target, err := os.Readlink(linkPath)
			if err != nil {
				return nil, fmt.Errorf("read global skill link: %w", err)
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(linkPath), target)
			}
			target, err = filepath.Abs(target)
			if err != nil {
				return nil, fmt.Errorf("resolve global skill target: %w", err)
			}
			if !pathWithin(target, storeDir) {
				continue
			}
			if isGlobalSkillException(entry.Name()) {
				continue
			}
			links = append(links, GlobalSymlink{
				Tool:          tool,
				Skill:         entry.Name(),
				Path:          linkPath,
				CanonicalPath: target,
			})
		}
	}

	sort.Slice(links, func(i, j int) bool {
		if links[i].Tool != links[j].Tool {
			return links[i].Tool < links[j].Tool
		}
		return links[i].Skill < links[j].Skill
	})
	return links, nil
}

// DiscoverCandidateProjects combines known state projection projects with
// search roots containing .scribe.yaml. If a search root has no nested project
// file, the root itself is still a candidate.
func DiscoverCandidateProjects(searchRoots []string, st *state.State) ([]ProjectCandidate, error) {
	candidates := map[string]ProjectCandidate{}

	for _, root := range searchRoots {
		if root == "" {
			continue
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, fmt.Errorf("resolve project search root: %w", err)
		}
		info, err := os.Stat(abs)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("stat project search root: %w", err)
		}
		if !info.IsDir() {
			abs = filepath.Dir(abs)
		}

		foundInRoot := false
		err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && path != abs && shouldSkipProjectWalkDir(d.Name()) {
				return filepath.SkipDir
			}
			if d.IsDir() || d.Name() != projectfile.Filename {
				return nil
			}
			foundInRoot = true
			dir := filepath.Dir(path)
			candidates[dir] = ProjectCandidate{Path: dir, Source: ".scribe.yaml"}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scan project candidates: %w", err)
		}
		if !foundInRoot {
			candidates[abs] = ProjectCandidate{Path: abs, Source: "search_root"}
		}
	}

	if st != nil {
		for _, skill := range st.Installed {
			for _, projection := range skill.Projections {
				if projection.Project == "" {
					continue
				}
				abs, err := filepath.Abs(projection.Project)
				if err != nil {
					return nil, fmt.Errorf("resolve projected project: %w", err)
				}
				if _, ok := candidates[abs]; !ok {
					candidates[abs] = ProjectCandidate{Path: abs, Source: "state"}
				}
			}
		}
	}

	out := make([]ProjectCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, candidate)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Path < out[j].Path
	})
	return out, nil
}

func pathWithin(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if path == root {
		return true
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}

func shouldSkipProjectWalkDir(name string) bool {
	switch name {
	case ".git", ".hg", ".svn", ".anvil", "node_modules", "vendor":
		return true
	default:
		return false
	}
}

func uniqueSkills(links []GlobalSymlink) []string {
	seen := map[string]bool{}
	for _, link := range links {
		seen[link.Skill] = true
	}
	skills := make([]string, 0, len(seen))
	for skill := range seen {
		skills = append(skills, skill)
	}
	sort.Strings(skills)
	return skills
}
