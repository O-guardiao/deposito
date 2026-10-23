BIN := bin/fisio

.PHONY: build run demo test vet fmt tidy clean css css-watch

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

# CSS: Tailwind v4 compila internal/web/styles/app.css -> internal/web/static/app.css
# (o arquivo gerado é commitado e embutido no binário; só precisa de Node quem mexe no estilo)
css:
	npm install --no-audit --no-fund && npm run css

css-watch:
	npm run css:watch
