# Used by `sam build` (BuildMethod: makefile in template.yaml).
build-FlashcardFunction:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o $(ARTIFACTS_DIR)/bootstrap ./cmd/lambda
