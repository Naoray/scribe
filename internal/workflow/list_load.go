package workflow

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Naoray/scribe/internal/discovery"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

// ListRow is the UI-agnostic row shape used by list surfaces.
type ListRow struct {
	Name      string
	Group     string
	Status    sync.Status
	HasStatus bool
	Version   string
	Author    string
	Targets   []string
	Local     *discovery.Skill
	Entry     *manifest.Entry
	LatestSHA string
	Excerpt   string
	Managed   bool
	Origin    state.Origin
}

func BuildRows(ctx context.Context, bag *Bag) ([]ListRow, []string, error) {
	localSkills, err := discovery.OnDisk(bag.State)
	if err != nil {
		return nil, nil, err
	}
	if !bag.RemoteFlag {
		return BuildLocalRows(localSkills, bag.State), nil, nil
	}

	localByName := make(map[string]*discovery.Skill, len(localSkills))
	for i := range localSkills {
		sk := &localSkills[i]
		localByName[sk.Name] = sk
	}

	repos := bag.Config.TeamRepos()
	if len(repos) == 0 {
		return BuildLocalRows(localSkills, bag.State), nil, nil
	}
	if bag.FilterRegistries != nil {
		filtered, ferr := bag.FilterRegistries(bag.RepoFlag, repos)
		if ferr != nil {
			return nil, nil, ferr
		}
		repos = filtered
	}

	syncer := &sync.Syncer{
		Client:   sync.WrapGitHubClient(bag.Client),
		Provider: bag.Provider,
		Tools:    []tools.Tool{},
	}

	matchedLocal := make(map[string]bool, len(localSkills))
	var rows []ListRow
	var warnings []string
	for _, repo := range repos {
		if bag.State.RegistryFailure(repo).Muted {
			continue
		}
		statuses, _, derr := syncer.Diff(ctx, repo, bag.State)
		if derr != nil {
			failure, changed := bag.State.RecordRegistryFailure(repo, derr, registryMuteAfter)
			if changed {
				bag.MarkStateDirty()
			}
			if !failure.Muted {
				warnings = append(warnings, fmt.Sprintf("%s: %v", repo, derr))
			}
			continue
		}
		if bag.State.ClearRegistryFailure(repo) {
			bag.MarkStateDirty()
		}
		for _, ss := range statuses {
			local := localByName[ss.Name]
			if local != nil {
				matchedLocal[ss.Name] = true
			}
			row := ListRow{
				Name:      ss.Name,
				Group:     repo,
				Status:    ss.Status,
				HasStatus: true,
				Version:   ss.DisplayVersion(),
				Author:    ss.Maintainer,
				Local:     local,
				Entry:     ss.Entry,
				LatestSHA: ss.LatestSHA,
			}
			if local != nil {
				row.Managed = local.Managed
			}
			if installed, ok := bag.State.Installed[ss.Name]; ok {
				row.Origin = installed.Origin
			}
			if ss.Installed != nil {
				row.Targets = ss.Installed.Tools
			}
			if local != nil && local.LocalPath != "" {
				row.Excerpt = readExcerpt(local.LocalPath, 8)
			}
			rows = append(rows, row)
		}
	}

	rows = append(rows, BuildLocalRowsExcluding(localSkills, matchedLocal, bag.State)...)
	return rows, warnings, nil
}

func BuildLocalRows(skills []discovery.Skill, st *state.State) []ListRow {
	var managedRows []ListRow
	var unmanagedRows []ListRow
	for i := range skills {
		sk := &skills[i]
		g := RegistryGroupFromName(sk.Name)
		row := ListRow{
			Name:    sk.Name,
			Group:   g,
			Targets: sk.Targets,
			Local:   sk,
			Managed: sk.Managed,
		}
		if installed, ok := st.Installed[sk.Name]; ok {
			row.Origin = installed.Origin
			if len(installed.Sources) > 0 && installed.Sources[0].Registry != "" {
				row.Group = installed.Sources[0].Registry
			}
		}
		if sk.LocalPath != "" {
			row.Excerpt = readExcerpt(sk.LocalPath, 8)
		}
		if row.Managed {
			managedRows = append(managedRows, row)
		} else {
			unmanagedRows = append(unmanagedRows, row)
		}
	}

	sortLocalRows(managedRows)
	sortLocalRows(unmanagedRows)

	rows := make([]ListRow, 0, len(managedRows)+len(unmanagedRows))
	rows = append(rows, managedRows...)
	rows = append(rows, unmanagedRows...)
	return rows
}

func RegistryGroupFromName(name string) string {
	if idx := strings.Index(name, "/"); idx > 0 {
		prefix := name[:idx]
		if prefix == "local" {
			return ""
		}
		return prefix
	}
	return ""
}

func sortLocalRows(rows []ListRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Group != rows[j].Group {
			if rows[i].Group == "" {
				return false
			}
			if rows[j].Group == "" {
				return true
			}
			return rows[i].Group < rows[j].Group
		}
		return rows[i].Name < rows[j].Name
	})
}

func BuildLocalRowsExcluding(skills []discovery.Skill, matched map[string]bool, st *state.State) []ListRow {
	var remaining []discovery.Skill
	for _, sk := range skills {
		if matched[sk.Name] {
			continue
		}
		remaining = append(remaining, sk)
	}
	return BuildLocalRows(remaining, st)
}

func readExcerpt(skillDir string, maxLines int) string {
	f, err := os.Open(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	pastFrontmatter := false
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if !pastFrontmatter {
			if trimmed == "---" {
				if !inFrontmatter {
					inFrontmatter = true
					continue
				}
				pastFrontmatter = true
				continue
			}
			if inFrontmatter {
				continue
			}
			pastFrontmatter = true
		}
		if trimmed == "" {
			if len(lines) > 0 && lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}
			continue
		}
		lines = append(lines, line)
		contentLines := 0
		for _, existing := range lines {
			if strings.TrimSpace(existing) != "" {
				contentLines++
			}
		}
		if contentLines >= maxLines {
			break
		}
	}
	return strings.Join(lines, "\n")
}

func NormalizeExcerptLine(line string) string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimLeft(trimmed, "#")
	trimmed = strings.TrimSpace(trimmed)

	replacer := strings.NewReplacer(
		"**", "",
		"__", "",
		"`", "",
	)
	trimmed = replacer.Replace(trimmed)

	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
		trimmed = strings.TrimSpace(trimmed[2:])
	}

	return trimmed
}
