# Build and package the Lambda. Deploy with AWS SAM (see template.yaml):
#   make zip
#   sam deploy --guided   (uses template.yaml, uploads function.zip)

BINARY := bootstrap   # name required by the provided.al2023 custom runtime

.PHONY: build zip test vet clean

build:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BINARY) ./cmd/server

zip: build
	zip -q function.zip $(BINARY)

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f $(BINARY) function.zip
