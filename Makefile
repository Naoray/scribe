docs:
	go run ./internal/cli/schema/cmd/gen-claudemd

check-docs:
	go generate ./...
	git diff --exit-code CLAUDE.md
