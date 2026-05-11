package cmd

import (
	"testing"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/kit"
)

func TestKitPushValidatesRegistryPublishableRefs(t *testing.T) {
	err := validateRegistryKitRefs(&kit.Kit{Name: "baseline", Skills: []string{"local:private"}}, "acme/skills")
	if clierrors.ExitCode(err) != clierrors.ExitValid {
		t.Fatalf("exit = %d, want valid; err=%v", clierrors.ExitCode(err), err)
	}
}

func TestNewKitCommandIncludesPush(t *testing.T) {
	root := newKitCommand()
	for _, cmd := range root.Commands() {
		if cmd.Name() == "push" {
			return
		}
	}
	t.Fatal("kit push command not registered")
}
