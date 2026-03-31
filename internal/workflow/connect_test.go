package workflow_test

import (
	"testing"

	"github.com/Naoray/scribe/internal/workflow"
)

func TestConnectSteps_EndsWithSyncTail(t *testing.T) {
	connect := workflow.ConnectSteps()
	tail := workflow.SyncTail()

	if len(connect) < len(tail) {
		t.Fatal("ConnectSteps shorter than SyncTail")
	}

	// The last steps should match the sync tail names.
	offset := len(connect) - len(tail)
	for i, step := range tail {
		if connect[offset+i].Name != step.Name {
			t.Errorf("connect[%d] = %q, want %q (from SyncTail)", offset+i, connect[offset+i].Name, step.Name)
		}
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
	if tail[0].Name != "DedupCheck" {
		t.Errorf("expected ConnectTail to start with DedupCheck, got %s", tail[0].Name)
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
