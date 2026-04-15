package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"

	"github.com/Naoray/scribe/internal/adopt"
	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/discovery"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

type tickSpinnerMsg struct{}
type rowsLoadedMsg struct {
	rows     []listRow
	warnings []string
}
type loadErrMsg struct{ err error }
type registryStatusesLoadedMsg struct {
	statuses map[string][]sync.SkillStatus
	warnings []string
}
type clipboardTickMsg struct{ id int }
type editorDoneMsg struct{ err error }
type commandDoneMsg struct{ err error }
type updateDoneMsg struct {
	name       string
	err        error
	merged     bool
	conflicted bool
	openPath   string
}
type toolsSavedMsg struct {
	result skillEditResult
	err    error
}
type repairSavedMsg struct {
	result skillProjectionRepairResult
	err    error
}

func (m listModel) runAdopt(row listRow) tea.Cmd {
	if row.Local == nil {
		return nil
	}
	bag := m.bag
	return func() tea.Msg {
		candidates, _, err := adopt.FindCandidates(bag.State, bag.Config.Adoption)
		if err != nil {
			return commandDoneMsg{err: err}
		}

		var target *adopt.Candidate
		for i := range candidates {
			if candidates[i].Name == row.Name && candidates[i].LocalPath == row.Local.LocalPath {
				target = &candidates[i]
				break
			}
		}
		if target == nil {
			return commandDoneMsg{err: fmt.Errorf("skill %q is not adoptable", row.Name)}
		}

		resolvedTools, err := tools.ResolveActive(bag.Config)
		if err != nil {
			return commandDoneMsg{err: err}
		}

		adopter := &adopt.Adopter{
			State: bag.State,
			Tools: resolvedTools,
		}
		result := adopter.Apply([]adopt.Candidate{*target})
		if err := result.Failed[target.Name]; err != nil {
			return commandDoneMsg{err: err}
		}
		return commandDoneMsg{}
	}
}

func (m listModel) runSaveTools(name string) tea.Cmd {
	cfg := m.bag.Config
	st := m.bag.State
	mode := m.toolMode
	desired := append([]string(nil), m.selectedToolNames()...)
	return func() tea.Msg {
		result, err := applySkillToolSelection(cfg, st, name, mode, desired)
		return toolsSavedMsg{result: result, err: err}
	}
}

func (m listModel) runRepair(name string) tea.Cmd {
	cfg := m.bag.Config
	st := m.bag.State
	return func() tea.Msg {
		result, err := repairSkillProjections(cfg, st, name)
		return repairSavedMsg{result: result, err: err}
	}
}

func (m listModel) runInstall(row listRow) tea.Cmd {
	if row.Entry == nil {
		return nil
	}
	ctx := m.ctx
	bag := m.bag
	return func() tea.Msg {
		if err := listEnsureRemoteDepsFn(ctx, bag); err != nil {
			return commandDoneMsg{err: err}
		}
		err := runAddDirectInstall(
			ctx,
			row.Group,
			row.Name,
			bag.Config,
			bag.State,
			newInstallSyncer(bag.Client, bag.Tools),
			bag.Client != nil && bag.Client.IsAuthenticated(),
			false,
			true,
		)
		return commandDoneMsg{err: err}
	}
}

func (m listModel) runUpdate(choice updateChoice) tea.Cmd {
	if m.cursor >= len(m.filtered) {
		return nil
	}

	row := m.filtered[m.cursor]
	repo := row.Group
	ss := sync.SkillStatus{
		Name:      row.Name,
		Status:    row.Status,
		Entry:     row.Entry,
		IsPackage: row.Entry.IsPackage(),
		LatestSHA: row.LatestSHA,
	}
	if row.Local != nil {
		if inst, ok := m.bag.State.Installed[row.Name]; ok {
			ss.Installed = &inst
		}
	}

	wasModified := rowHasLocalModifications(row, m.bag.State)

	ctx := m.ctx
	bag := m.bag
	return func() tea.Msg {
		if err := listEnsureRemoteDepsFn(ctx, bag); err != nil {
			return updateDoneMsg{name: row.Name, err: err}
		}
		syncer := &sync.Syncer{
			Client:           sync.WrapGitHubClient(bag.Client),
			Provider:         bag.Provider,
			Tools:            bag.Tools,
			Executor:         &sync.ShellExecutor{},
			TrustAll:         bag.TrustAllFlag,
			ModifiedStrategy: sync.ModifiedStrategyMerge,
		}
		if choice == updateChoicePreferTheirs {
			syncer.ModifiedStrategy = sync.ModifiedStrategyPreferTheirs
		}
		isTTY := isatty.IsTerminal(os.Stdin.Fd())
		if isTTY && !bag.TrustAllFlag && !bag.JSONFlag {
			syncer.ApprovalFunc = func(name, command, source string) bool {
				var approved bool
				err := huh.NewConfirm().
					Title(fmt.Sprintf("Package %q wants to run a shell command", name)).
					Description(fmt.Sprintf("source:  %s\ncommand: %s", source, command)).
					Affirmative("Approve").
					Negative("Deny").
					Value(&approved).
					Run()
				if err != nil {
					return false
				}
				return approved
			}
		}
		err := syncer.RunWithDiff(ctx, repo, []sync.SkillStatus{ss}, bag.State)
		if err != nil {
			return updateDoneMsg{name: row.Name, err: err}
		}
		localSkills, _ := discovery.OnDisk(bag.State)
		for _, sk := range localSkills {
			if sk.Name == row.Name && sk.Conflicted {
				return updateDoneMsg{
					name:       row.Name,
					conflicted: true,
					openPath:   filepath.Join(sk.LocalPath, "SKILL.md"),
				}
			}
		}
		return updateDoneMsg{name: row.Name, merged: wasModified && choice == updateChoiceMerge}
	}
}

func rowHasLocalModifications(row listRow, st *state.State) bool {
	if st == nil {
		return false
	}
	if row.Local != nil {
		if row.Local.Modified {
			return true
		}
		if row.Local.LocalPath != "" {
			installed, ok := st.Installed[row.Name]
			if !ok {
				return false
			}
			return sync.IsLocallyModified(row.Local.LocalPath, installed.InstalledHash)
		}
	}
	installed, ok := st.Installed[row.Name]
	if !ok {
		return false
	}
	storeDir, err := tools.StoreDir()
	if err != nil {
		return false
	}
	return sync.IsLocallyModified(filepath.Join(storeDir, row.Name), installed.InstalledHash)
}

func resolveEditor(cfg *config.Config) string {
	if cfg != nil && cfg.Editor != "" {
		return cfg.Editor
	}
	if e := os.Getenv("VISUAL"); e != "" {
		return e
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return "vi"
}
