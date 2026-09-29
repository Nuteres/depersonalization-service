APP_NAME = depersonalizer
VERSION = 1.0.0

.PHONY: all build-all build-altlinux build-windows build-macos test clean

all: build-all

build-all: build-altlinux build-windows build-macos

# Сборка под Alt Linux 10.4 внутри Docker с нативной привязкой glibc 2.32
build-altlinux:
	@echo "==> Сборка под Alt Linux 10.4 (Docker alt:p10)..."
	@mkdir -p bin
	docker build -f build/altlinux/Dockerfile -t depersonalizer-builder-alt:p10 .
	docker run --rm -v $(PWD)/bin:/app/bin depersonalizer-builder-alt:p10

# Сборка под Windows x86_64
build-windows:
	@echo "==> Сборка под Windows (x86_64)..."
	@mkdir -p bin
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -H windowsgui" -o bin/$(APP_NAME)-windows-amd64.exe ./cmd/app

# Сборка под macOS (Universal Binary)
build-macos:
	@echo "==> Сборка под macOS Universal (ARM64 + Intel)..."
	@mkdir -p bin
	GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o bin/$(APP_NAME)-darwin-arm64 ./cmd/app
	GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o bin/$(APP_NAME)-darwin-amd64 ./cmd/app
	lipo -create -output bin/$(APP_NAME)-darwin-universal bin/$(APP_NAME)-darwin-arm64 bin/$(APP_NAME)-darwin-amd64
	rm bin/$(APP_NAME)-darwin-arm64 bin/$(APP_NAME)-darwin-amd64

test:
	go test -v ./pkg/...

clean:
	rm -rf bin/
