APP_NAME := verificador-citas
BUILD_DIR := bin
MAIN_PACKAGE := ./cmd/server
GO := go
GOCACHE ?= /tmp/verificador-go-build-cache
GO_RUN := GOCACHE=$(GOCACHE) $(GO)
GOFMT_FILES := $(shell find cmd internal -type f -name '*.go')

.DEFAULT_GOAL := help

.PHONY: help serve run dev build test fmt check docker-build clean

help: ## Muestra los comandos disponibles
	@awk 'BEGIN {FS = ":.*## "}; /^[a-zA-Z_-]+:.*## / {printf "\033[36m%-12s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

serve: ## Levanta el servidor en desarrollo
	@mkdir -p $(BUILD_DIR)
	$(GO_RUN) run $(MAIN_PACKAGE)

run: serve ## Alias de serve

dev: serve ## Alias de serve

build: ## Genera el binario local en ./bin
	@mkdir -p $(BUILD_DIR)
	$(GO_RUN) build -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PACKAGE)

test: ## Ejecuta la suite de tests
	$(GO_RUN) test ./...

fmt: ## Formatea el codigo Go
	gofmt -w $(GOFMT_FILES)

check: fmt test build ## Ejecuta formato, tests y build

docker-build: ## Construye la imagen Docker local
	docker build -t $(APP_NAME):local .

clean: ## Elimina artefactos de build sin tocar data/
	rm -rf $(BUILD_DIR) server
