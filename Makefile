APP_NAME = depersonalizer
VERSION = 1.8.2

.PHONY: all build-all build-frontend build-altlinux build-windows build-macos test clean

all: build-all

build-all: build-altlinux build-windows build-macos

# Сборка Svelte 5 фронтенда через Vite
build-frontend:
	@echo "==> Сборка Svelte 5 фронтенда (Vite)..."
	npm --prefix frontend run build

# Сборка под Alt Linux 10.4 внутри Docker с нативной привязкой glibc 2.32 (x86_64)
build-altlinux: build-frontend
	@echo "==> Сборка под Alt Linux 10.4 (Docker alt:p10 x86_64)..."
	@mkdir -p bin
	docker build --platform linux/amd64 -f build/altlinux/Dockerfile -t depersonalizer-builder-alt:p10 .
	docker run --platform linux/amd64 --rm -v $(CURDIR)/bin:/app/bin depersonalizer-builder-alt:p10

# Сборка под Windows x86_64 с внедрением иконки и манифеста
build-windows: build-frontend
	@echo "==> Генерация Windows ресурсов и иконки (.syso)..."
	go run build/tools/build_syso.go
	@echo "==> Сборка под Windows (x86_64)..."
	@mkdir -p bin
	GOOS=windows GOARCH=amd64 go build -mod=vendor -tags desktop,production -ldflags="-s -w -H windowsgui" -o bin/$(APP_NAME)-windows-amd64.exe ./cmd/app

# Сборка под macOS (Universal Binary + .app Bundle)
build-macos: build-frontend
	@echo "==> Сборка под macOS Universal (ARM64 + Intel)..."
	@mkdir -p bin
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -mod=vendor -tags desktop,production -ldflags="-s -w -extldflags '-framework UniformTypeIdentifiers'" -o bin/$(APP_NAME)-darwin-arm64 ./cmd/app
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -mod=vendor -tags desktop,production -ldflags="-s -w -extldflags '-framework UniformTypeIdentifiers'" -o bin/$(APP_NAME)-darwin-amd64 ./cmd/app
	lipo -create -output bin/$(APP_NAME)-darwin-universal bin/$(APP_NAME)-darwin-arm64 bin/$(APP_NAME)-darwin-amd64
	rm bin/$(APP_NAME)-darwin-arm64 bin/$(APP_NAME)-darwin-amd64
	@echo "==> Оформление macOS .app бандла и установка иконки..."
	swift build/tools/bundle_macos.swift

test: build-frontend
	go test -v ./...

clean:
	rm -rf bin/ frontend/dist/
