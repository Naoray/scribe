package envelope

type contextKey string

const (
	BootstrapStartKey contextKey = "bootstrap_start"
	RunEStartKey      contextKey = "runE_start"
	CommandPathKey    contextKey = "command_path"
	ScribeVersionKey  contextKey = "scribe_version"
	DurationMSKey     contextKey = "duration_ms"
	BootstrapMSKey    contextKey = "bootstrap_ms"
)
