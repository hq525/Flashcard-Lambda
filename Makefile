# Used by `sam build` (BuildMethod: makefile in template.yaml).
build-FlashcardFunction:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o $(ARTIFACTS_DIR)/bootstrap ./cmd/lambda

# Authenticated loopback dev server, loading resource/auth settings from .env.
run:
	set -a && . ./.env && set +a && go run ./cmd/server -addr 127.0.0.1:8080
