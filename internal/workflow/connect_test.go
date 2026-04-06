package workflow_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/workflow"
)

func TestConnectSteps_EndsWithSyncSkills(t *testing.T) {
	connect := workflow.ConnectSteps()

	// Connect should end with ResolveTools → SyncSkills (the sync execution
	// steps). ResolveFormatter is promoted earlier in connect so the formatter
	// is available for connect-specific output before sync begins.
	last := connect[len(connect)-1]
	if last.Name != "SyncSkills" {
		t.Errorf("expected last step SyncSkills, got %s", last.Name)
	}
	secondLast := connect[len(connect)-2]
	if secondLast.Name != "ResolveTools" {
		t.Errorf("expected second-to-last step ResolveTools, got %s", secondLast.Name)
	}
}

func TestConnectSteps_StartsWithLoadConfig(t *testing.T) {
	steps := workflow.ConnectSteps()
	if steps[0].Name != "LoadConfig" {
		t.Errorf("expected first step LoadConfig, got %s", steps[0].Name)
	}
}

func TestConnectTail_SkipsLoadConfig(t *testing.T) {
	tail := workflow.ConnectTail()
	if tail[0].Name == "LoadConfig" {
		t.Error("ConnectTail should not start with LoadConfig")
	}
	if tail[0].Name != "ResolveFormatter" {
		t.Errorf("expected ConnectTail to start with ResolveFormatter, got %s", tail[0].Name)
	}
}

func TestConnectSteps_ContainsDedupCheck(t *testing.T) {
	steps := workflow.ConnectSteps()
	found := false
	for _, s := range steps {
		if s.Name == "DedupCheck" {
			found = true
			break
		}
	}
	if !found {
		t.Error("ConnectSteps missing DedupCheck step")
	}
}
