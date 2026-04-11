package app

import (
	"context"
	"sync"

	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/state"
)

// Factory lazily constructs command dependencies so help/version paths avoid
// config I/O, state reads, and GitHub client setup entirely.
type Factory struct {
	Config   func() (*config.Config, error)
	State    func() (*state.State, error)
	Client   func() (*gh.Client, error)
	Provider func() (provider.Provider, error)
}

func NewFactory() *Factory {
	f := &Factory{}

	f.Config = sync.OnceValues(config.Load)
	f.State = sync.OnceValues(state.Load)
	f.Client = sync.OnceValues(func() (*gh.Client, error) {
		cfg, err := f.Config()
		if err != nil {
			return nil, err
		}
		return gh.NewClient(context.Background(), cfg.Token), nil
	})
	f.Provider = sync.OnceValues(func() (provider.Provider, error) {
		client, err := f.Client()
		if err != nil {
			return nil, err
		}
		return provider.NewGitHubProvider(provider.WrapGitHubClient(client)), nil
	})

	return f
}
