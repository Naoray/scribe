package source

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

type SourceType string

const (
	SourceGitHub SourceType = "github"
	SourceGitLab SourceType = "gitlab"
	SourceGit    SourceType = "git"
	SourceLocal  SourceType = "local"
)

type SourceSpec struct {
	ID       string     `yaml:"id,omitempty" json:"id,omitempty"`
	Type     SourceType `yaml:"type" json:"type"`
	Repo     string     `yaml:"repo,omitempty" json:"repo,omitempty"`
	URL      string     `yaml:"url,omitempty" json:"url,omitempty"`
	Ref      string     `yaml:"ref,omitempty" json:"ref,omitempty"`
	Path     string     `yaml:"path,omitempty" json:"path,omitempty"`
	Host     string     `yaml:"host,omitempty" json:"host,omitempty"`
	Writable *bool      `yaml:"writable,omitempty" json:"writable,omitempty"`
}

type SourceIdentity struct {
	Key          string `json:"key"`
	Type         string `json:"type"`
	Locator      string `json:"locator"`
	Ref          string `json:"ref,omitempty"`
	Path         string `json:"path,omitempty"`
	ResolvedRev  string `json:"resolved_rev,omitempty"`
	ContentScope string `json:"content_scope,omitempty"`
}

type InstallRef struct {
	Source SourceSpec
	Skill  string
}

var ownerRepoPattern = regexp.MustCompile(`^[^/\s]+/[^/\s]+$`)

func ParseSourceArg(arg string) (SourceSpec, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return SourceSpec{}, fmt.Errorf("source is empty")
	}
	if ownerRepoPattern.MatchString(arg) && !looksLikePath(arg) {
		return CanonicalSpec(SourceSpec{Type: SourceGitHub, Repo: strings.TrimSuffix(arg, ".git")})
	}
	if strings.HasPrefix(arg, "file://") {
		u, err := url.Parse(arg)
		if err != nil {
			return SourceSpec{}, fmt.Errorf("invalid file URL %q: %w", arg, err)
		}
		return CanonicalSpec(SourceSpec{Type: SourceLocal, Path: u.Path})
	}
	if looksLikeLocal(arg) {
		return CanonicalSpec(SourceSpec{Type: SourceLocal, Path: arg})
	}
	if isGitURL(arg) {
		return parseGitURL(arg)
	}
	if strings.Contains(arg, "://") {
		return SourceSpec{}, fmt.Errorf("unsupported source URL %q", arg)
	}
	return SourceSpec{}, fmt.Errorf("invalid source %q", arg)
}

func ParseInstallRef(ref string) (InstallRef, error) {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "[") {
		end := strings.LastIndex(ref, "]:")
		if end < 0 {
			return InstallRef{}, fmt.Errorf("invalid install ref %q: expected [source]:skill", ref)
		}
		spec, err := ParseSourceArg(ref[1:end])
		if err != nil {
			return InstallRef{}, err
		}
		skill := strings.TrimSpace(ref[end+2:])
		if skill == "" {
			return InstallRef{}, fmt.Errorf("invalid install ref %q: skill name is empty", ref)
		}
		return InstallRef{Source: spec, Skill: skill}, nil
	}
	if strings.Contains(ref, "://") {
		return InstallRef{}, fmt.Errorf("invalid install ref %q: URL sources must use [source]:skill or --source", ref)
	}
	idx := strings.LastIndex(ref, ":")
	if idx < 0 {
		return InstallRef{}, fmt.Errorf("invalid install ref %q: expected owner/repo:skill or [source]:skill", ref)
	}
	spec, err := ParseSourceArg(ref[:idx])
	if err != nil {
		return InstallRef{}, err
	}
	if spec.Type != SourceGitHub || spec.Path != "" {
		return InstallRef{}, fmt.Errorf("invalid install ref %q: unbracketed refs must use owner/repo:skill", ref)
	}
	skill := strings.TrimSpace(ref[idx+1:])
	if skill == "" {
		return InstallRef{}, fmt.Errorf("invalid install ref %q: skill name is empty", ref)
	}
	return InstallRef{Source: spec, Skill: skill}, nil
}

func Canonicalize(spec SourceSpec) (SourceSpec, SourceIdentity, error) {
	canonical, err := CanonicalSpec(spec)
	if err != nil {
		return SourceSpec{}, SourceIdentity{}, err
	}
	key := ""
	locator := ""
	switch canonical.Type {
	case SourceGitHub:
		host := canonical.Host
		if host == "" {
			host = "github.com"
		}
		locator = canonical.Repo
		key = "github:" + strings.ToLower(canonical.Repo)
		if canonical.Path != "" {
			key += ":" + canonical.Path
		}
		_ = host
	case SourceGitLab:
		host := canonical.Host
		if host == "" {
			host = "gitlab.com"
		}
		locator = host + "/" + canonical.Repo
		key = "gitlab:" + strings.ToLower(locator)
		if canonical.Path != "" {
			key += ":" + canonical.Path
		}
	case SourceGit:
		locator = canonical.URL
		key = "git:" + canonical.URL
		if canonical.Path != "" {
			key += ":" + canonical.Path
		}
	case SourceLocal:
		locator = canonical.Path
		key = "local:" + canonical.Path
	default:
		return SourceSpec{}, SourceIdentity{}, fmt.Errorf("invalid source type %q", spec.Type)
	}
	return canonical, SourceIdentity{
		Key:          key,
		Type:         string(canonical.Type),
		Locator:      locator,
		Ref:          canonical.Ref,
		Path:         canonical.Path,
		ContentScope: canonical.Path,
	}, nil
}

func CanonicalSpec(spec SourceSpec) (SourceSpec, error) {
	spec.ID = strings.TrimSpace(spec.ID)
	spec.Type = SourceType(strings.ToLower(strings.TrimSpace(string(spec.Type))))
	spec.Repo = strings.Trim(strings.TrimSpace(spec.Repo), "/")
	spec.URL = strings.TrimSpace(spec.URL)
	spec.Ref = strings.TrimSpace(spec.Ref)
	spec.Host = strings.ToLower(strings.TrimSpace(spec.Host))

	var err error
	switch spec.Type {
	case SourceGitHub:
		spec.Repo = strings.TrimSuffix(spec.Repo, ".git")
		if spec.Host == "" {
			spec.Host = "github.com"
		}
		if spec.Repo == "" {
			return SourceSpec{}, fmt.Errorf("github source requires repo")
		}
		if !validRepo(spec.Repo, false) {
			return SourceSpec{}, fmt.Errorf("invalid github repo %q: expected owner/repo", spec.Repo)
		}
		if spec.URL == "" {
			spec.URL = "https://github.com/" + spec.Repo
		}
		spec.Path, err = cleanGitPath(spec.Path)
	case SourceGitLab:
		if spec.Host == "" {
			spec.Host = "gitlab.com"
		}
		if spec.Repo == "" {
			return SourceSpec{}, fmt.Errorf("gitlab source requires repo")
		}
		if !validRepo(spec.Repo, true) {
			return SourceSpec{}, fmt.Errorf("invalid gitlab repo %q", spec.Repo)
		}
		if spec.URL == "" {
			spec.URL = "https://" + spec.Host + "/" + spec.Repo
		}
		spec.Path, err = cleanGitPath(spec.Path)
	case SourceGit:
		if spec.URL == "" {
			return SourceSpec{}, fmt.Errorf("git source requires url")
		}
		spec.URL = normalizeGitURL(spec.URL)
		spec.Path, err = cleanGitPath(spec.Path)
	case SourceLocal:
		if spec.Path == "" {
			return SourceSpec{}, fmt.Errorf("local source requires path")
		}
		if spec.Ref != "" {
			return SourceSpec{}, fmt.Errorf("local source does not support ref")
		}
		if spec.Repo != "" || spec.URL != "" {
			return SourceSpec{}, fmt.Errorf("local source uses path, not repo or url")
		}
		spec.Path, err = cleanLocalPath(spec.Path)
	default:
		return SourceSpec{}, fmt.Errorf("invalid source type %q", spec.Type)
	}
	if err != nil {
		return SourceSpec{}, err
	}
	return spec, nil
}

func parseGitURL(raw string) (SourceSpec, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return SourceSpec{}, fmt.Errorf("invalid source URL %q: %w", raw, err)
	}
	host := strings.ToLower(u.Hostname())
	switch {
	case host == "github.com" && (u.Scheme == "http" || u.Scheme == "https"):
		return parseGitHubURL(raw, u)
	case host == "gitlab.com" && (u.Scheme == "http" || u.Scheme == "https"):
		return parseGitLabURL(raw, u)
	default:
		return CanonicalSpec(SourceSpec{Type: SourceGit, URL: raw})
	}
}

func parseGitHubURL(raw string, u *url.URL) (SourceSpec, error) {
	parts := cleanURLParts(u.Path)
	if len(parts) < 2 {
		return SourceSpec{}, fmt.Errorf("invalid GitHub URL %q: expected github.com/owner/repo", raw)
	}
	spec := SourceSpec{
		Type: SourceGitHub,
		Repo: parts[0] + "/" + strings.TrimSuffix(parts[1], ".git"),
		URL:  "https://github.com/" + parts[0] + "/" + strings.TrimSuffix(parts[1], ".git"),
	}
	if len(parts) > 2 {
		if parts[2] != "tree" && parts[2] != "blob" {
			return SourceSpec{}, fmt.Errorf("invalid GitHub URL %q: expected repo or tree URL", raw)
		}
		if len(parts) < 4 {
			return SourceSpec{}, fmt.Errorf("invalid GitHub URL %q: missing ref", raw)
		}
		spec.Ref = parts[3]
		if len(parts) > 4 {
			spec.Path = strings.Join(parts[4:], "/")
		}
	}
	return CanonicalSpec(spec)
}

func parseGitLabURL(raw string, u *url.URL) (SourceSpec, error) {
	parts := cleanURLParts(u.Path)
	if len(parts) < 2 {
		return SourceSpec{}, fmt.Errorf("invalid GitLab URL %q: expected gitlab.com/group/project", raw)
	}
	spec := SourceSpec{Type: SourceGitLab, Host: "gitlab.com"}
	treeIdx := -1
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "-" && (parts[i+1] == "tree" || parts[i+1] == "blob") {
			treeIdx = i
			break
		}
	}
	repoEnd := len(parts)
	if treeIdx >= 0 {
		repoEnd = treeIdx
	}
	spec.Repo = strings.Join(parts[:repoEnd], "/")
	spec.URL = "https://gitlab.com/" + spec.Repo
	if treeIdx >= 0 {
		if len(parts) <= treeIdx+2 {
			return SourceSpec{}, fmt.Errorf("invalid GitLab URL %q: missing ref", raw)
		}
		spec.Ref = parts[treeIdx+2]
		if len(parts) > treeIdx+3 {
			spec.Path = strings.Join(parts[treeIdx+3:], "/")
		}
	}
	return CanonicalSpec(spec)
}

func cleanURLParts(rawPath string) []string {
	cleaned := path.Clean("/" + rawPath)
	trimmed := strings.Trim(cleaned, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func cleanGitPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" || p == "." {
		return "", nil
	}
	if path.IsAbs(p) {
		return "", fmt.Errorf("source path %q must be relative", p)
	}
	if strings.Contains(p, "\\") {
		return "", fmt.Errorf("source path %q must use slash separators", p)
	}
	cleaned := path.Clean(p)
	if cleaned == "." {
		return "", nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return "", fmt.Errorf("source path %q cannot traverse directories", p)
	}
	for _, part := range strings.Split(cleaned, "/") {
		if part == "" || part == "." || part == ".." || part == ".git" {
			return "", fmt.Errorf("invalid source path %q", p)
		}
	}
	return cleaned, nil
}

func cleanLocalPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("local source requires path")
	}
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get home dir: %w", err)
		}
		p = filepath.Join(home, p[2:])
	}
	if !filepath.IsAbs(p) {
		abs, err := filepath.Abs(p)
		if err != nil {
			return "", fmt.Errorf("resolve local path %q: %w", p, err)
		}
		p = abs
	}
	return filepath.Clean(p), nil
}

func validRepo(repo string, allowNested bool) bool {
	parts := strings.Split(repo, "/")
	if (!allowNested && len(parts) != 2) || (allowNested && len(parts) < 2) {
		return false
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, " \t\n\r") {
			return false
		}
	}
	return true
}

func isGitURL(s string) bool {
	return strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "ssh://") ||
		strings.HasPrefix(s, "git+ssh://") ||
		strings.HasPrefix(s, "git://")
}

func normalizeGitURL(s string) string {
	if strings.HasPrefix(s, "git+ssh://") {
		return "ssh://" + strings.TrimPrefix(s, "git+ssh://")
	}
	return strings.TrimSuffix(s, "/")
}

func looksLikeLocal(s string) bool {
	return strings.HasPrefix(s, "~/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") || filepath.IsAbs(s)
}

func looksLikePath(s string) bool {
	return strings.HasPrefix(s, ".") || strings.HasPrefix(s, "~") || filepath.IsAbs(s)
}
