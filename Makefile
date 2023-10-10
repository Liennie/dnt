build: main

targets := $(shell find ./internal -name '*.go')

main: $(targets) go.mod main.go
	go build main.go

run: main
	./main 152a11c2-d972-4942-814c-e30e71b7c84f
