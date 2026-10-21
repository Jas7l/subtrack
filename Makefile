.PHONY: install.dep lint vet test build run clean

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

clean:
	go clean
	rm -rf $(BUILD_DIR)
