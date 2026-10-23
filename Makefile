BIN := bin/fisio

.PHONY: build run demo test vet fmt tidy clean

build:
	go build -o $(BIN) ./cmd/fisio

run: build
	$(BIN)

demo: build
	$(BIN) -demo

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

tidy:
	go mod tidy

clean:
	rm -rf bin
