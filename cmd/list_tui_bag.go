package cmd

import (
	"context"
	"fmt"
	"sort"
	stdsync "sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
	"github.com/Naoray/scribe/internal/workflow"
)

const registryMuteAfter = 3
const registryStatusCacheTTL = 5 * time.Minute

var listRegistryStatusesFn = loadRegistryStatuses
var listEnsureRemoteDepsFn = ensureListRemoteDepsLoaded
var nowFn = time.Now

type cachedRegistryStatuses struct {
	at       time.Time
	statuses []sync.SkillStatus
}

var registryStatusCache = struct {
	mu    stdsync.Mutex
	items map[string]cachedRegistryStatuses
}{
	items: map[string]cachedRegistryStatuses{},
}

func loadRowsCmd(ctx context.Context, bag *workflow.Bag) tea.Cmd {
	return func() tea.Msg {
		if err := ensureListBagLoaded(ctx, bag); err != nil {
			return loadErrMsg{err: err}
		}
		rows, warnings, err := workflow.BuildRows(ctx, bag)
		if err != nil {
			return loadErrMsg{err: err}
		}
		if err := saveWorkflowState(bag); err != nil {
			return loadErrMsg{err: err}
		}
		return rowsLoadedMsg{rows: rows, warnings: warnings}
	}
}

func ensureListBagLoaded(ctx context.Context, bag *workflow.Bag) error {
	if bag == nil {
		return fmt.Errorf("list loader: missing workflow bag")
	}
	needsTools := bag.RemoteFlag || bag.BrowseFlag
	if bag.Config != nil && bag.State != nil && (!needsTools || bag.Tools != nil) {
		return nil
	}

	steps := workflow.ListLoadStepsLocal()
	if needsTools {
		steps = workflow.ListLoadStepsRemote()
	}
	if err := workflow.Run(ctx, steps, bag); err != nil {
		return err
	}
	return saveWorkflowState(bag)
}

func ensureListRemoteDepsLoaded(ctx context.Context, bag *workflow.Bag) error {
	if bag == nil {
		return fmt.Errorf("list action: missing workflow bag")
	}
	if bag.Config == nil || bag.State == nil {
		if err := ensureListBagLoaded(ctx, bag); err != nil {
			return err
		}
	}
	if bag.Provider != nil && bag.Tools != nil {
		return nil
	}

	originalLazy := bag.LazyGitHub
	bag.LazyGitHub = false
	defer func() {
		bag.LazyGitHub = originalLazy
	}()

	if err := workflow.Run(ctx, []workflow.Step{
		{Name: "LoadConfig", Fn: workflow.StepLoadConfig},
		{Name: "LoadState", Fn: workflow.StepLoadState},
		{Name: "ResolveTools", Fn: workflow.StepResolveTools},
	}, bag); err != nil {
		return err
	}
	return saveWorkflowState(bag)
}

func loadRegistryStatusesCmd(ctx context.Context, bag *workflow.Bag, repos []string) tea.Cmd {
	return func() tea.Msg {
		statuses, warnings := listRegistryStatusesFn(ctx, bag, repos)
		return registryStatusesLoadedMsg{statuses: statuses, warnings: warnings}
	}
}

func registriesForBackgroundCheck(cfg *config.Config, st *state.State) []string {
	if cfg == nil || st == nil {
		return nil
	}
	enabled := make(map[string]bool, len(cfg.TeamRepos()))
	for _, repo := range cfg.TeamRepos() {
		enabled[repo] = true
	}
	set := map[string]bool{}
	for _, installed := range st.Installed {
		if installed.Origin != state.OriginRegistry {
			continue
		}
		for _, src := range installed.Sources {
			if src.Registry != "" && enabled[src.Registry] {
				set[src.Registry] = true
			}
		}
	}
	repos := make([]string, 0, len(set))
	for repo := range set {
		repos = append(repos, repo)
	}
	sort.Strings(repos)
	return repos
}

func loadRegistryStatuses(ctx context.Context, bag *workflow.Bag, repos []string) (map[string][]sync.SkillStatus, []string) {
	if bag == nil || bag.Factory == nil {
		return nil, nil
	}
	client, err := bag.Factory.Client()
	if err != nil {
		return nil, []string{fmt.Sprintf("load github client: %v", err)}
	}
	prov := provider.NewGitHubProvider(provider.WrapGitHubClient(client))
	syncer := &sync.Syncer{
		Client:   sync.WrapGitHubClient(client),
		Provider: prov,
		Tools:    []tools.Tool{},
	}
	statusesByRepo := make(map[string][]sync.SkillStatus, len(repos))
	var warnings []string
	for _, repo := range repos {
		if cached, ok := loadCachedRegistryStatuses(repo); ok {
			statusesByRepo[repo] = cached
			continue
		}
		statuses, _, err := syncer.Diff(ctx, repo, bag.State)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", repo, err))
			continue
		}
		storeCachedRegistryStatuses(repo, statuses)
		statusesByRepo[repo] = statuses
	}
	return statusesByRepo, warnings
}

func loadCachedRegistryStatuses(repo string) ([]sync.SkillStatus, bool) {
	registryStatusCache.mu.Lock()
	defer registryStatusCache.mu.Unlock()
	cached, ok := registryStatusCache.items[repo]
	if !ok {
		return nil, false
	}
	if nowFn().Sub(cached.at) > registryStatusCacheTTL {
		delete(registryStatusCache.items, repo)
		return nil, false
	}
	return cached.statuses, true
}

func storeCachedRegistryStatuses(repo string, statuses []sync.SkillStatus) {
	registryStatusCache.mu.Lock()
	defer registryStatusCache.mu.Unlock()
	copied := make([]sync.SkillStatus, len(statuses))
	copy(copied, statuses)
	registryStatusCache.items[repo] = cachedRegistryStatuses{
		at:       nowFn(),
		statuses: copied,
	}
}
