GOOS = $(shell go env GOOS)
GOARCH = $(shell go env GOARCH)
CURRENT_DIR := $(shell pwd)
# PROJECT_NAME := $(notdir $(CURRENT_DIR))
PROJECT_NAME := baton-file
ifeq ($(GOOS),windows)
OUTPUT_PATH = ./bin/${PROJECT_NAME}.exe
else
OUTPUT_PATH = ./bin/${PROJECT_NAME}
endif

.PHONY: build build-linux build-macos build-windows test clean run build-all

build:
	go build -mod=mod -o ${OUTPUT_PATH} ./cmd/${PROJECT_NAME}/main.go

# Build for Linux
build-linux:
	GOOS=linux GOARCH=amd64 go build -mod=mod -o ./bin/${PROJECT_NAME}-linux-amd64 ./cmd/${PROJECT_NAME}/main.go
	GOOS=linux GOARCH=arm64 go build -mod=mod -o ./bin/${PROJECT_NAME}-linux-arm64 ./cmd/${PROJECT_NAME}/main.go

# Build for macOS
build-macos:
	GOOS=darwin GOARCH=amd64 go build -mod=mod -o ./bin/${PROJECT_NAME}-darwin-amd64 ./cmd/${PROJECT_NAME}/main.go
	GOOS=darwin GOARCH=arm64 go build -mod=mod -o ./bin/${PROJECT_NAME}-darwin-arm64 ./cmd/${PROJECT_NAME}/main.go

# Build for Windows
build-windows:
	GOOS=windows GOARCH=amd64 go build -mod=mod -o ./bin/${PROJECT_NAME}.exe ./cmd/${PROJECT_NAME}/main.go

# Build for all platforms
build-all: build-linux build-macos build-windows

# Run the application
run:
	${OUTPUT_PATH} --config-file=./config.yaml

# Clean build artifacts
clean:
	rm -rf bin/

# Run tests
test:
	go test -v ./...