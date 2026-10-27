.PHONY: install.dep lint vet test build run clean swagger seed web-install web-dev web-build

PROJECTNAME ?= subtrack
PROJECT_VERSION ?= 0.0.0
BUILD_DIR ?= bin

install.dep:
	$(info ************ INSTALLING DEPENDENCIES  ************)
	go mod download

lint:
	$(info ************ LINT CHECKING  ************)
	#https://golangci-lint.run/welcome/install/
	golangci-lint run

vet:
	go vet ./...

test:
	go test ./...

# Build the package (./cmd/server), never the file (./cmd/server/main.go):
# naming a .go file compiles only that file and ignores the rest of the package.
build:
	$(info ************ BUILD  ************)
	go build -ldflags "-X 'main.Version=$(PROJECT_VERSION)'" -o $(BUILD_DIR)/$(PROJECTNAME) ./cmd/server

run: build
	$(BUILD_DIR)/$(PROJECTNAME)

# seed fills an empty database with demo data through the API of a running
# server (see cmd/seed).
seed:
	go run ./cmd/seed

clean:
	go clean
	rm -rf $(BUILD_DIR)

# swagger regenerates docs/ (the OpenAPI spec the server serves on /swagger/)
# from the annotations: the general info on main() in cmd/server/main.go and
# a godoc block on every handler. Run it after changing a route or a DTO and
# commit docs/ with the change; TestSpecMatchesRouter fails otherwise.
SWAG ?= go run github.com/swaggo/swag/cmd/swag@v1.16.6

swagger:
	$(SWAG) fmt -g cmd/server/main.go
	$(SWAG) init -g cmd/server/main.go -o docs --parseInternal --outputTypes go,json,yaml

# Web client (web/). NODE_BIN is prepended to PATH for the case when Node is
# unpacked outside PATH, e.g. ~/.local/node.
NODE_BIN ?= $(HOME)/.local/node/bin
WEB_ENV = PATH="$(NODE_BIN):$$PATH"

web-install:
	cd web && $(WEB_ENV) npm ci

# Vite on :5173, proxies /api to the server on :8080 (see web/vite.config.ts).
web-dev:
	cd web && $(WEB_ENV) npm run dev

web-build:
	cd web && $(WEB_ENV) npm run build
