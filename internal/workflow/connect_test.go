package workflow_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/workflow"
)

func TestConnectSteps_EndsWithShowAvailable(t *testing.T) {
	steps := workflow.ConnectSteps()
	last := steps[len(steps)-1]
	if last.Name != "ShowAvailable" {
		t.Errorf("expected last step ShowAvailable, got %s (connect must not auto-install)", last.Name)
	}
}

func TestConnectSteps_StartsWithLoadConfig(t *testing.T) {
	steps := workflow.ConnectSteps()
	if steps[0].Name != "LoadConfig" {
		t.Errorf("expected first step LoadConfig, got %s", steps[0].Name)
	}
}

func TestConnectSteps_ContainsDedupCheck(t *testing.T) {
	steps := workflow.ConnectSteps()
	for _, s := range steps {
		if s.Name == "DedupCheck" {
			return
		}
	}
	t.Error("ConnectSteps missing DedupCheck step")
}

func TestConnectSteps_DoesNotContainSyncSkills(t *testing.T) {
	for _, s := range workflow.ConnectSteps() {
		if s.Name == "SyncSkills" {
			t.Error("ConnectSteps must not contain SyncSkills — connect is opt-in, not auto-install")
		}
	}
}

func TestConnectInstallAllSteps_ContainsSyncSkills(t *testing.T) {
	steps := workflow.ConnectInstallAllSteps()
	last := steps[len(steps)-1]
	if last.Name != "SyncSkills" {
		t.Errorf("expected ConnectInstallAllSteps last step SyncSkills, got %s", last.Name)
	}
	if steps[0].Name != "LoadConfig" {
		t.Errorf("expected ConnectInstallAllSteps to start with LoadConfig, got %s", steps[0].Name)
	}
}

func TestConnectInstallAllTail_SkipsLoadConfig(t *testing.T) {
	tail := workflow.ConnectInstallAllTail()
	if tail[0].Name == "LoadConfig" {
		t.Error("ConnectInstallAllTail should not start with LoadConfig")
	}
	if tail[0].Name != "ResolveFormatter" {
		t.Errorf("expected ConnectInstallAllTail to start with ResolveFormatter, got %s", tail[0].Name)
	}
}

func TestConnectInstallAllTail_EndsWithSyncSkills(t *testing.T) {
	tail := workflow.ConnectInstallAllTail()
	last := tail[len(tail)-1]
	if last.Name != "SyncSkills" {
		t.Errorf("expected ConnectInstallAllTail last step SyncSkills, got %s", last.Name)
	}
}
