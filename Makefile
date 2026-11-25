.PHONY: all test race fuzz bench cli app examples ui-dev check
all: check test cli

check:
	gofmt -l . && go vet ./...
	cd desktop && go vet .
	cd desktop/frontend && npm run check

test:
	go test -race ./...
	cd desktop && go test -race .
	cd desktop/frontend && npm test

fuzz:
	go test ./format -run '^$$' -fuzz '^FuzzReader$$' -fuzztime 60s
	go test ./format -run '^$$' -fuzz '^FuzzEventLine$$' -fuzztime 60s
	go test ./redact -run '^$$' -fuzz '^FuzzRedactString$$' -fuzztime 60s
	go test ./otlp -run '^$$' -fuzz '^FuzzImport$$' -fuzztime 60s

bench:
	scripts/bench.sh | tee docs/bench-raw.txt

cli:
	go build -trimpath -o bin/repropack ./cmd/repropack

app:
	cd desktop && wails build -platform darwin/arm64

examples:
	go run ./internal/genexamples
	./bin/repropack html spec/examples/checkout-crash.repropack -o spec/examples/checkout-crash.html

ui-dev:
	cd desktop/frontend && npm run dev
