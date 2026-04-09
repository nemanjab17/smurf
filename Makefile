BINARY_DIR   := bin
SMURF_BIN    := $(BINARY_DIR)/smurf
SMURFD_BIN   := $(BINARY_DIR)/smurfd
GO           := go
GOFLAGS      := -trimpath
LDFLAGS      := -s -w

.PHONY: all build smurf smurfd clean test lint proto

all: build

build: smurf smurfd

smurf:
	@mkdir -p $(BINARY_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(SMURF_BIN) ./cmd/smurf

smurfd:
	@mkdir -p $(BINARY_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(SMURFD_BIN) ./cmd/smurfd

install: build
	install -m 755 $(SMURF_BIN)  /usr/local/bin/smurf
	install -m 755 $(SMURFD_BIN) /usr/local/bin/smurfd
	install -m 644 init/smurfd.service /etc/systemd/system/smurfd.service
	systemctl daemon-reload

clean:
	rm -rf $(BINARY_DIR)

test:
	$(GO) test ./... -count=1 -race

test-unit:
	$(GO) test ./... -count=1 -race -tags "!integration !e2e"

test-integration:
	$(GO) test ./... -count=1 -tags integration -v

lint:
	golangci-lint run ./...

# Generate protobuf code (requires protoc + plugins)
proto:
	protoc \
		--go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		api/proto/smurf.proto
	@echo "NOTE: After generating, replace api/smurfv1/*.go with the generated output"

# Host setup (run once on a fresh KVM-capable Linux machine)
setup-host:
	@sudo bash scripts/setup-host.sh

# Build the base rootfs image
build-rootfs:
	@sudo bash scripts/build-rootfs.sh

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

.DEFAULT_GOAL := build
