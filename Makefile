LAMBDA_BUILD_DIR := .build/lambda
LAMBDA_ZIP := dist/grass-lambda.zip

.PHONY: lambda-package build-GrassFunction web-build build web-test

web-build:
	npm --prefix web ci
	npm --prefix web run build
	test -f webserver/assets/dist/index.html

build: web-build
	mkdir -p .build
	go build -o .build/grass .

web-test:
	npm --prefix web run typecheck
	npm --prefix web test
	npm --prefix web run test:e2e

lambda-package:
	mkdir -p "$(LAMBDA_BUILD_DIR)" "$(dir $(LAMBDA_ZIP))"
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o "$(LAMBDA_BUILD_DIR)/bootstrap" ./cmd/lambda
	rm -f "$(LAMBDA_ZIP)"
	cd "$(LAMBDA_BUILD_DIR)" && zip -q "$(abspath $(LAMBDA_ZIP))" bootstrap

build-GrassFunction:
	mkdir -p "$(ARTIFACTS_DIR)"
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o "$(ARTIFACTS_DIR)/bootstrap" ./cmd/lambda
