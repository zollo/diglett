VERSION ?= dev
IMAGE ?= ghcr.io/zollo/diglett
BIN ?= diglett

.PHONY: build run test vet fmt tidy docker clean

build: ## Build the diglett binary
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BIN) .

run: ## Run locally on :8080
	go run .

test: ## Run the test suite with the race detector
	go test -race ./...

vet: ## Static analysis
	go vet ./...

fmt: ## Format the code
	gofmt -w .

tidy: ## Tidy module dependencies
	go mod tidy

docker: ## Build the container image
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) -t $(IMAGE):latest .

clean:
	rm -f $(BIN)
