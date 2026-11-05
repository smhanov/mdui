.PHONY: run test lint

run:
	go run . $(ARGS)

test:
	go test ./...

lint:
	go vet ./...
