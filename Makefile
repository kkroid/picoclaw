.PHONY: all build install uninstall clean help test build-appfactory-builder verify-appfactory-builder verify-appfactory-template verify-appfactory-template-fast check-appfactory-template-governance refresh-appfactory-notifications check-appfactory-notification-governance apply-appfactory-notification-auto-ack verify-appfactory-public-job verify-appfactory-public-job-fast verify-appfactory-public-job-device verify-appfactory-public-job-device-fast verify-appfactory-public-job-device-summary check-appfactory-public-job-device-alerts run-appfactory-public-job-device-regression run-appfactory-public-job-device-regression-fast run-appfactory-public-job-device-regression-pool-fast update-appfactory-public-job-device-regression-index update-appfactory-public-job-device-pool-status verify-appfactory-product-flow verify-appfactory-product-flow-summary check-appfactory-product-flow-alerts run-appfactory-product-flow-regression verify-appfactory-jobs-regression run-appfactory-jobs-regression verify-appfactory-jobs-auto-repair verify-appfactory-jobs-live-auto-repair-probe verify-appfactory-platform-regression-summary check-appfactory-platform-regression-alerts check-appfactory-internal-trial-freshness verify-appfactory-internal-trial-status run-appfactory-platform-regression validate-builder-runtime-ollama validate-builder-runtime-ollama-samples validate-builder-runtime-ollama-sandbox validate-builder-runtime-ollama-real verify-appfactory-builder-runtime-auto-repair summarize-builder-runtime-validation

# Build variables
BINARY_NAME=picoclaw
BUILD_DIR=build
CMD_DIR=cmd/$(BINARY_NAME)
MAIN_GO=$(CMD_DIR)/main.go

# Version
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT=$(shell git rev-parse --short=8 HEAD 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date +%FT%T%z)
GO_VERSION=$(shell $(GO) version | awk '{print $$3}')
CONFIG_PKG=github.com/sipeed/picoclaw/pkg/config
LDFLAGS=-X $(CONFIG_PKG).Version=$(VERSION) -X $(CONFIG_PKG).GitCommit=$(GIT_COMMIT) -X $(CONFIG_PKG).BuildTime=$(BUILD_TIME) -X $(CONFIG_PKG).GoVersion=$(GO_VERSION) -s -w

# Go variables
GO?=CGO_ENABLED=0 go
WEB_GO?=$(GO)
GO_BUILD_TAGS?=goolm,stdjson
GOFLAGS?=-v -tags $(GO_BUILD_TAGS)
GO_SOURCE_PACKAGES=./cmd/... ./pkg/... ./examples/...
comma:=,
empty:=
space:=$(empty) $(empty)
GO_BUILD_TAGS_NO_GOOLM:=$(subst $(space),$(comma),$(strip $(filter-out goolm,$(subst $(comma),$(space),$(GO_BUILD_TAGS)))))
GOFLAGS_NO_GOOLM?=-v -tags $(GO_BUILD_TAGS_NO_GOOLM)
APPFACTORY_BUILDER_PROXY?=
APPFACTORY_BUILDER_ADD_HOST?=--add-host=host.docker.internal:host-gateway
APPFACTORY_BUILDER_IMAGE?=picoclaw/appfactory-builder:local
APPFACTORY_BUILDER_BUILD_ARGS=$(APPFACTORY_BUILDER_ADD_HOST) $(if $(strip $(APPFACTORY_BUILDER_PROXY)),--build-arg HTTP_PROXY=$(APPFACTORY_BUILDER_PROXY) --build-arg HTTPS_PROXY=$(APPFACTORY_BUILDER_PROXY) --build-arg ALL_PROXY=$(APPFACTORY_BUILDER_PROXY),) --build-arg NO_PROXY=127.0.0.1,localhost,host.docker.internal
APPFACTORY_BUILDER_DOCKER_NETWORK?=

# Patch MIPS LE ELF e_flags (offset 36) for NaN2008-only kernels (e.g. Ingenic X2600).
#
# Bytes (octal): \004 \024 \000 \160  →  little-endian 0x70001404
#   0x70000000  EF_MIPS_ARCH_32R2   MIPS32 Release 2
#   0x00001000  EF_MIPS_ABI_O32     O32 ABI
#   0x00000400  EF_MIPS_NAN2008     IEEE 754-2008 NaN encoding
#   0x00000004  EF_MIPS_CPIC        PIC calling sequence
#
# Go's GOMIPS=softfloat emits no FP instructions, so the NaN mode is irrelevant
# at runtime — this is purely an ELF metadata fix to satisfy the kernel's check.
# patchelf cannot modify e_flags; dd at a fixed offset is the most portable way.
#
# Ref: https://codebrowser.dev/linux/linux/arch/mips/include/asm/elf.h.html
define PATCH_MIPS_FLAGS
	@if [ -f "$(1)" ]; then \
		printf '\004\024\000\160' | dd of=$(1) bs=1 seek=36 count=4 conv=notrunc 2>/dev/null || \
		{ echo "Error: failed to patch MIPS e_flags for $(1)"; exit 1; }; \
	else \
		echo "Error: $(1) not found, cannot patch MIPS e_flags"; exit 1; \
	fi
endef

# Patch creack/pty for loong64 support (upstream doesn't have ztypes_loong64.go)
PTY_PATCH_LOONG64=pty_dir=$$(go env GOMODCACHE)/github.com/creack/pty@v1.1.9; \
	if [ -d "$$pty_dir" ] && [ ! -f "$$pty_dir/ztypes_loong64.go" ]; then \
		chmod +w "$$pty_dir" 2>/dev/null || true; \
		printf '//go:build linux && loong64\npackage pty\ntype (_C_int int32; _C_uint uint32)\n' > "$$pty_dir/ztypes_loong64.go"; \
	fi

# Golangci-lint
GOLANGCI_LINT?=golangci-lint

# Installation
INSTALL_PREFIX?=$(HOME)/.local
INSTALL_BIN_DIR=$(INSTALL_PREFIX)/bin
INSTALL_MAN_DIR=$(INSTALL_PREFIX)/share/man/man1
INSTALL_TMP_SUFFIX=.new

# Workspace and Skills
PICOCLAW_HOME?=$(HOME)/.picoclaw
WORKSPACE_DIR?=$(PICOCLAW_HOME)/workspace
WORKSPACE_SKILLS_DIR=$(WORKSPACE_DIR)/skills
BUILTIN_SKILLS_DIR=$(CURDIR)/skills

# OS detection
UNAME_S:=$(shell uname -s)
UNAME_M:=$(shell uname -m)

# Platform-specific settings
ifeq ($(UNAME_S),Linux)
	PLATFORM=linux
	ifeq ($(UNAME_M),x86_64)
		ARCH=amd64
	else ifeq ($(UNAME_M),aarch64)
		ARCH=arm64
	else ifeq ($(UNAME_M),armv81)
		ARCH=arm64
	else ifeq ($(UNAME_M),loongarch64)
		ARCH=loong64
	else ifeq ($(UNAME_M),riscv64)
		ARCH=riscv64
	else ifeq ($(UNAME_M),mipsel)
		ARCH=mipsle
	else
		ARCH=$(UNAME_M)
	endif
else ifeq ($(UNAME_S),Darwin)
	PLATFORM=darwin
	WEB_GO=CGO_ENABLED=1 go
	ifeq ($(UNAME_M),x86_64)
		ARCH=amd64
	else ifeq ($(UNAME_M),arm64)
		ARCH=arm64
	else
		ARCH=$(UNAME_M)
	endif
else
	PLATFORM=$(UNAME_S)
	ARCH=$(UNAME_M)
endif

BINARY_PATH=$(BUILD_DIR)/$(BINARY_NAME)-$(PLATFORM)-$(ARCH)

# Default target
all: build

## generate: Run generate
generate:
	@echo "Run generate..."
	@rm -r ./$(CMD_DIR)/workspace 2>/dev/null || true
	@$(GO) generate ./cmd/picoclaw/internal/onboard
	@echo "Run generate complete"

## build: Build the picoclaw binary for current platform
build: generate
	@echo "Building $(BINARY_NAME) for $(PLATFORM)/$(ARCH)..."
	@mkdir -p $(BUILD_DIR)
	@$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY_PATH) ./$(CMD_DIR)
	@echo "Build complete: $(BINARY_PATH)"
	@ln -sf $(BINARY_NAME)-$(PLATFORM)-$(ARCH) $(BUILD_DIR)/$(BINARY_NAME)

## build-launcher: Build the picoclaw-launcher (web console) binary
build-launcher:
	@echo "Building picoclaw-launcher for $(PLATFORM)/$(ARCH)..."
	@mkdir -p $(BUILD_DIR)
	@echo "Building embedded frontend assets..."
	@cd web/frontend && pnpm install && pnpm build:backend
	@$(WEB_GO) build $(GOFLAGS) -o $(BUILD_DIR)/picoclaw-launcher-$(PLATFORM)-$(ARCH) ./web/backend
	@ln -sf picoclaw-launcher-$(PLATFORM)-$(ARCH) $(BUILD_DIR)/picoclaw-launcher
	@echo "Build complete: $(BUILD_DIR)/picoclaw-launcher"

## build-launcher-tui: Build the picoclaw-launcher TUI binary
build-launcher-tui:
	@echo "Building picoclaw-launcher-tui for $(PLATFORM)/$(ARCH)..."
	@mkdir -p $(BUILD_DIR)
	@$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/picoclaw-launcher-tui-$(PLATFORM)-$(ARCH) ./cmd/picoclaw-launcher-tui
	@ln -sf picoclaw-launcher-tui-$(PLATFORM)-$(ARCH) $(BUILD_DIR)/picoclaw-launcher-tui
	@echo "Build complete: $(BUILD_DIR)/picoclaw-launcher-tui"

## build-appfactory-builder: Build the Flutter/Android builder image used for appfactory validation
build-appfactory-builder:
	@echo "Building appfactory builder image..."
	@docker build $(APPFACTORY_BUILDER_BUILD_ARGS) -f docker/Dockerfile.appfactory-builder -t $(APPFACTORY_BUILDER_IMAGE) .
	@echo "Build complete: $(APPFACTORY_BUILDER_IMAGE)"

## verify-appfactory-builder: Run Flutter and Android toolchain checks inside the builder image
verify-appfactory-builder: build-appfactory-builder
	@echo "Running appfactory builder smoke checks..."
	@docker run --rm $(APPFACTORY_BUILDER_IMAGE) smoke

## verify-appfactory-template: Run Flutter analyze/test/build against the template seed inside the builder image
verify-appfactory-template: build-appfactory-builder
	@echo "Running appfactory template validation..."
	@bash scripts/verify-appfactory-template.sh

## verify-appfactory-template-fast: Run template seed smoke validation without APK build, reusing persistent builder caches
verify-appfactory-template-fast: build-appfactory-builder
	@echo "Running fast appfactory template validation without APK build..."
	@APPFACTORY_TEMPLATE_VERIFY_INCLUDE_APK=0 bash scripts/verify-appfactory-template.sh

## check-appfactory-template-governance: Evaluate template governance signals such as license evidence, dependency risk, and manifest permissions
check-appfactory-template-governance:
	@echo "Checking appfactory template governance..."
	@bash scripts/check-appfactory-template-governance.sh

## refresh-appfactory-notifications: Rebuild the persisted public notifications snapshot from the backend API
refresh-appfactory-notifications:
	@echo "Refreshing appfactory notifications snapshot..."
	@bash scripts/refresh-appfactory-notifications.sh

## check-appfactory-notification-governance: Classify notification backlog, emit governance summary, and optionally auto-ack safe stale items
check-appfactory-notification-governance:
	@echo "Checking appfactory notification governance..."
	@bash scripts/check-appfactory-notification-governance.sh

## apply-appfactory-notification-auto-ack: Apply safe notification auto-ack and allow persisted-snapshot fallback when backend API is unavailable
apply-appfactory-notification-auto-ack:
	@echo "Applying appfactory notification safe auto-ack..."
	@APPFACTORY_NOTIFICATION_GOVERNANCE_APPLY_AUTO_ACK=1 APPFACTORY_NOTIFICATION_GOVERNANCE_ALLOW_OFFLINE_SNAPSHOT_ACK=1 bash scripts/check-appfactory-notification-governance.sh

## verify-appfactory-public-job: Generate a real public-job Flutter workspace, then run Flutter analyze/test/build inside the builder image
verify-appfactory-public-job: build-appfactory-builder
	@echo "Running appfactory public job validation..."
	@VERIFY_APPFACTORY_ENTRYPOINT="make verify-appfactory-public-job" bash scripts/verify-appfactory-public-job.sh

## verify-appfactory-public-job-fast: Re-run the default public-job Flutter validation against an existing builder image without rebuilding it
verify-appfactory-public-job-fast:
	@echo "Running appfactory public job validation without rebuilding the builder image..."
	@VERIFY_APPFACTORY_ENTRYPOINT="make verify-appfactory-public-job-fast" bash scripts/verify-appfactory-public-job.sh

## verify-appfactory-public-job-device: Run public-job Flutter validation with optional adb install/launch/logcat capture enabled
verify-appfactory-public-job-device: build-appfactory-builder
	@echo "Running appfactory public job device validation..."
	@VERIFY_APPFACTORY_ENTRYPOINT="make verify-appfactory-public-job-device" APPFACTORY_DEVICE_VERIFICATION_ENABLED=1 bash scripts/verify-appfactory-public-job.sh

## verify-appfactory-public-job-device-fast: Re-run the public-job device validation against an existing builder image without rebuilding it
verify-appfactory-public-job-device-fast:
	@echo "Running appfactory public job device validation without rebuilding the builder image..."
	@VERIFY_APPFACTORY_ENTRYPOINT="make verify-appfactory-public-job-device-fast" APPFACTORY_DEVICE_VERIFICATION_ENABLED=1 bash scripts/verify-appfactory-public-job.sh

## verify-appfactory-public-job-device-summary: Summarize device failure categories from historical metrics.json files
verify-appfactory-public-job-device-summary:
	@echo "Summarizing appfactory public job device failure metrics..."
	@bash scripts/update-appfactory-device-summary.sh

## validate-builder-runtime-ollama: Validate Ollama connectivity and minimal builder-runtime probes
validate-builder-runtime-ollama:
	@echo "Validating builder-runtime Ollama connectivity..."
	@bash scripts/validate-builder-runtime-ollama.sh

## validate-builder-runtime-ollama-samples: Run the first five builder-runtime leaf task sample probes against Ollama
validate-builder-runtime-ollama-samples:
	@echo "Running builder-runtime Ollama leaf task samples..."
	@bash scripts/run-builder-runtime-model-samples.sh

## validate-builder-runtime-ollama-sandbox: Generate and apply a real single-file WorkspacePatch inside a temporary sandbox
validate-builder-runtime-ollama-sandbox:
	@echo "Running builder-runtime Ollama sandbox patch validation..."
	@bash scripts/run-builder-runtime-patch-sandbox.sh

## validate-builder-runtime-ollama-real: Run container-backed real Flutter analyze/test repair validation against Ollama-generated patches
validate-builder-runtime-ollama-real:
	@echo "Running builder-runtime Ollama real Flutter validation..."
	@bash scripts/run-builder-runtime-real-validation.sh

## verify-appfactory-builder-runtime-auto-repair: Run the focused adapter integration test that exercises analyze auto repair and archive the evidence
verify-appfactory-builder-runtime-auto-repair:
	@echo "Running builder-runtime auto repair verification..."
	@bash scripts/run-appfactory-builder-runtime-auto-repair-regression.sh

## summarize-builder-runtime-validation: Summarize archived builder-runtime validation results across leaf samples, real validation, and sandbox runs
summarize-builder-runtime-validation:
	@echo "Summarizing builder-runtime validation results..."
	@bash scripts/summarize-builder-runtime-validation.sh

## check-appfactory-public-job-device-alerts: Refresh device summary and fail on configured device regression thresholds
check-appfactory-public-job-device-alerts:
	@echo "Checking appfactory public job device failure alerts..."
	@bash scripts/check-appfactory-device-alerts.sh

## run-appfactory-public-job-device-regression: Run full device validation, archive artifacts, refresh summaries, and evaluate alerts
run-appfactory-public-job-device-regression:
	@echo "Running full appfactory public job device regression cycle..."
	@APPFACTORY_DEVICE_REGRESSION_MODE=full bash scripts/run-appfactory-device-regression.sh

## run-appfactory-public-job-device-regression-fast: Run fast device validation, archive artifacts, refresh summaries, and evaluate alerts
run-appfactory-public-job-device-regression-fast:
	@echo "Running fast appfactory public job device regression cycle..."
	@APPFACTORY_DEVICE_REGRESSION_MODE=fast bash scripts/run-appfactory-device-regression.sh

## run-appfactory-public-job-device-regression-pool-fast: Run fast device regression with fixed device pool lease management enabled
run-appfactory-public-job-device-regression-pool-fast:
	@echo "Running fast appfactory public job device regression cycle with fixed device pool enabled..."
	@APPFACTORY_DEVICE_POOL_ENABLED=1 APPFACTORY_DEVICE_REGRESSION_MODE=fast bash scripts/run-appfactory-device-regression.sh

## update-appfactory-public-job-device-regression-index: Rebuild regression index.json and latest.md from archived regression results
update-appfactory-public-job-device-regression-index:
	@echo "Updating appfactory public job device regression index..."
	@bash scripts/update-appfactory-device-regression-index.sh

## update-appfactory-public-job-device-pool-status: Rebuild fixed device pool status.json and status.md
update-appfactory-public-job-device-pool-status:
	@echo "Updating appfactory public job device pool status..."
	@bash scripts/update-appfactory-device-pool-status.sh

## verify-appfactory-product-flow: Run the fixed product-level appfactory flow (requirement -> approval -> prepare -> run -> review -> delivery) against a live backend API
verify-appfactory-product-flow:
	@echo "Running appfactory product flow verification..."
	@bash scripts/run-appfactory-product-e2e.sh

## verify-appfactory-product-flow-summary: Summarize archived product-flow results across cold/warm cache runs
verify-appfactory-product-flow-summary:
	@echo "Summarizing appfactory product flow regression results..."
	@bash scripts/update-appfactory-product-flow-summary.sh

## check-appfactory-product-flow-alerts: Refresh product-flow summary and fail on configured runtime thresholds
check-appfactory-product-flow-alerts:
	@echo "Checking appfactory product flow alerts..."
	@bash scripts/check-appfactory-product-flow-alerts.sh

## run-appfactory-product-flow-regression: Run product-flow verification, refresh summaries, and evaluate alerts
run-appfactory-product-flow-regression:
	@echo "Running appfactory product flow regression cycle..."
	@bash scripts/run-appfactory-product-flow-regression.sh

## verify-appfactory-jobs-regression: Run the fixed /jobs-equivalent compile/create/start flow against a live backend API and archive the result
verify-appfactory-jobs-regression:
	@echo "Running appfactory /jobs regression verification..."
	@bash scripts/run-appfactory-jobs-regression.sh

## verify-appfactory-jobs-auto-repair: Run the public /jobs API integration test that exercises analyze auto repair and archive the evidence
verify-appfactory-jobs-auto-repair:
	@echo "Running appfactory /jobs auto repair verification..."
	@bash scripts/run-appfactory-jobs-auto-repair-regression.sh

## verify-appfactory-jobs-live-auto-repair-probe: Run the live /jobs regression path with a repair canary goal summary and human notes against the current backend
verify-appfactory-jobs-live-auto-repair-probe:
	@echo "Running appfactory live /jobs auto repair probe..."
	@bash scripts/run-appfactory-jobs-live-auto-repair-probe.sh

## run-appfactory-jobs-regression: Alias for the fixed /jobs regression entrypoint
run-appfactory-jobs-regression:
	@echo "Running appfactory /jobs regression cycle..."
	@bash scripts/run-appfactory-jobs-regression.sh

## verify-appfactory-platform-regression-summary: Summarize archived platform regression results and refresh latest status pages
verify-appfactory-platform-regression-summary:
	@echo "Summarizing appfactory platform regression results..."
	@bash scripts/update-appfactory-platform-regression-summary.sh

## check-appfactory-platform-regression-alerts: Refresh platform regression summary and fail on top-level stage regressions
check-appfactory-platform-regression-alerts:
	@echo "Checking appfactory platform regression alerts..."
	@bash scripts/check-appfactory-platform-regression-alerts.sh

## check-appfactory-internal-trial-freshness: Check whether product-flow, platform regression, and optional device regression latest results are still fresh enough for internal trial operations
check-appfactory-internal-trial-freshness:
	@echo "Checking appfactory internal trial freshness..."
	@bash scripts/check-appfactory-internal-trial-freshness.sh

## verify-appfactory-internal-trial-status: Refresh internal trial status dashboard from freshness and optional notifications snapshot
verify-appfactory-internal-trial-status:
	@echo "Updating appfactory internal trial status dashboard..."
	@bash scripts/update-appfactory-internal-trial-status.sh

## run-appfactory-platform-regression: Run the current top-level regression spine across product-flow, builder-runtime real validation, and optional device regression
run-appfactory-platform-regression:
	@echo "Running appfactory platform regression spine..."
	@bash scripts/run-appfactory-platform-regression.sh

## build-whatsapp-native: Build with WhatsApp native (whatsmeow) support; larger binary
build-whatsapp-native: generate
## @echo "Building $(BINARY_NAME) with WhatsApp native for $(PLATFORM)/$(ARCH)..."
	@echo "Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build -tags $(GO_BUILD_TAGS),whatsapp_native -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./$(CMD_DIR)
	GOOS=linux GOARCH=arm GOARM=7 $(GO) build -tags $(GO_BUILD_TAGS),whatsapp_native -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm ./$(CMD_DIR)
	GOOS=linux GOARCH=arm64 $(GO) build -tags $(GO_BUILD_TAGS),whatsapp_native -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./$(CMD_DIR)
	GOOS=linux GOARCH=loong64 $(GO) build -tags $(GO_BUILD_TAGS),whatsapp_native -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-loong64 ./$(CMD_DIR)
	GOOS=linux GOARCH=riscv64 $(GO) build -tags $(GO_BUILD_TAGS),whatsapp_native -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-riscv64 ./$(CMD_DIR)
	GOOS=linux GOARCH=mipsle GOMIPS=softfloat $(GO) build -tags $(GO_BUILD_TAGS_NO_GOOLM),whatsapp_native -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-mipsle ./$(CMD_DIR)
	$(call PATCH_MIPS_FLAGS,$(BUILD_DIR)/$(BINARY_NAME)-linux-mipsle)
	GOOS=darwin GOARCH=arm64 $(GO) build -tags $(GO_BUILD_TAGS),whatsapp_native -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./$(CMD_DIR)
	GOOS=windows GOARCH=amd64 $(GO) build -tags $(GO_BUILD_TAGS),whatsapp_native -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./$(CMD_DIR)
## @$(GO) build $(GOFLAGS) -tags whatsapp_native -ldflags "$(LDFLAGS)" -o $(BINARY_PATH) ./$(CMD_DIR)
	@echo "Build complete"
##	@ln -sf $(BINARY_NAME)-$(PLATFORM)-$(ARCH) $(BUILD_DIR)/$(BINARY_NAME)

## build-linux-arm: Build for Linux ARMv7 (e.g. Raspberry Pi Zero 2 W 32-bit)
build-linux-arm: generate
	@echo "Building for linux/arm (GOARM=7)..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm GOARM=7 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm ./$(CMD_DIR)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)-linux-arm"

## build-linux-arm64: Build for Linux ARM64 (e.g. Raspberry Pi Zero 2 W 64-bit)
build-linux-arm64: generate
	@echo "Building for linux/arm64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./$(CMD_DIR)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64"

## build-linux-mipsle: Build for Linux MIPS32 LE
build-linux-mipsle: generate
	@echo "Building for linux/mipsle (softfloat)..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=mipsle GOMIPS=softfloat $(GO) build $(GOFLAGS_NO_GOOLM) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-mipsle ./$(CMD_DIR)
	$(call PATCH_MIPS_FLAGS,$(BUILD_DIR)/$(BINARY_NAME)-linux-mipsle)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)-linux-mipsle"

## build-pi-zero: Build for Raspberry Pi Zero 2 W (32-bit and 64-bit)
build-pi-zero: build-linux-arm build-linux-arm64
	@echo "Pi Zero 2 W builds: $(BUILD_DIR)/$(BINARY_NAME)-linux-arm (32-bit), $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 (64-bit)"

## build-all: Build picoclaw for all platforms
build-all: generate
	@echo "Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./$(CMD_DIR)
	GOOS=linux GOARCH=arm GOARM=7 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm ./$(CMD_DIR)
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./$(CMD_DIR)
	@$(PTY_PATCH_LOONG64)
	GOOS=linux GOARCH=loong64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-loong64 ./$(CMD_DIR)
	GOOS=linux GOARCH=riscv64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-riscv64 ./$(CMD_DIR)
	GOOS=linux GOARCH=mipsle GOMIPS=softfloat $(GO) build $(GOFLAGS_NO_GOOLM) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-mipsle ./$(CMD_DIR)
	$(call PATCH_MIPS_FLAGS,$(BUILD_DIR)/$(BINARY_NAME)-linux-mipsle)
	GOOS=linux GOARCH=arm GOARM=7 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-armv7 ./$(CMD_DIR)
	GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./$(CMD_DIR)
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./$(CMD_DIR)
	GOOS=netbsd GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-netbsd-amd64 ./$(CMD_DIR)
	GOOS=netbsd GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-netbsd-arm64 ./$(CMD_DIR)
	@echo "All builds complete"

## install: Install picoclaw to system and copy builtin skills
install: build
	@echo "Installing $(BINARY_NAME)..."
	@mkdir -p $(INSTALL_BIN_DIR)
	# Copy binary with temporary suffix to ensure atomic update
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_BIN_DIR)/$(BINARY_NAME)$(INSTALL_TMP_SUFFIX)
	@chmod +x $(INSTALL_BIN_DIR)/$(BINARY_NAME)$(INSTALL_TMP_SUFFIX)
	@mv -f $(INSTALL_BIN_DIR)/$(BINARY_NAME)$(INSTALL_TMP_SUFFIX) $(INSTALL_BIN_DIR)/$(BINARY_NAME)
	@echo "Installed binary to $(INSTALL_BIN_DIR)/$(BINARY_NAME)"
	@echo "Installation complete!"

## uninstall: Remove picoclaw from system
uninstall:
	@echo "Uninstalling $(BINARY_NAME)..."
	@rm -f $(INSTALL_BIN_DIR)/$(BINARY_NAME)
	@echo "Removed binary from $(INSTALL_BIN_DIR)/$(BINARY_NAME)"
	@echo "Note: Only the executable file has been deleted."
	@echo "If you need to delete all configurations (config.json, workspace, etc.), run 'make uninstall-all'"

## uninstall-all: Remove picoclaw and all data
uninstall-all:
	@echo "Removing workspace and skills..."
	@rm -rf $(PICOCLAW_HOME)
	@echo "Removed workspace: $(PICOCLAW_HOME)"
	@echo "Complete uninstallation done!"

## clean: Remove build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@echo "Clean complete"

## vet: Run go vet for static analysis
vet: generate
	@$(GO) vet $(GOFLAGS) $(GO_SOURCE_PACKAGES)
	@cd web/backend && $(WEB_GO) vet ./...

## test: Test Go code
test: generate
	@$(GO) test $(GOFLAGS) $(GO_SOURCE_PACKAGES)
	@cd web && make test

## fmt: Format Go code
fmt:
	@$(GOLANGCI_LINT) fmt

## lint: Run linters
lint:
	@$(GOLANGCI_LINT) run --build-tags $(GO_BUILD_TAGS)

## fix: Fix linting issues
fix:
	@$(GOLANGCI_LINT) run --fix --build-tags $(GO_BUILD_TAGS)

## deps: Download dependencies
deps:
	@$(GO) mod download
	@$(GO) mod verify

## update-deps: Update dependencies
update-deps:
	@$(GO) get -u ./...
	@$(GO) mod tidy

## check: Run vet, fmt, and verify dependencies
check: deps fmt vet test

## run: Build and run picoclaw
run: build
	@$(BUILD_DIR)/$(BINARY_NAME) $(ARGS)

## docker-build: Build Docker image (minimal Alpine-based)
docker-build:
	@echo "Building minimal Docker image (Alpine-based)..."
	docker compose -f docker/docker-compose.yml build picoclaw-agent picoclaw-gateway

## docker-build-full: Build Docker image with full MCP support (Node.js 24)
docker-build-full:
	@echo "Building full-featured Docker image (Node.js 24)..."
	docker compose -f docker/docker-compose.full.yml build picoclaw-agent picoclaw-gateway

## docker-test: Test MCP tools in Docker container
docker-test:
	@echo "Testing MCP tools in Docker..."
	@chmod +x scripts/test-docker-mcp.sh
	@./scripts/test-docker-mcp.sh

## docker-run: Run picoclaw gateway in Docker (Alpine-based)
docker-run:
	docker compose -f docker/docker-compose.yml --profile gateway up

## docker-run-full: Run picoclaw gateway in Docker (full-featured)
docker-run-full:
	docker compose -f docker/docker-compose.full.yml --profile gateway up

## docker-run-agent: Run picoclaw agent in Docker (interactive, Alpine-based)
docker-run-agent:
	docker compose -f docker/docker-compose.yml run --rm picoclaw-agent

## docker-run-agent-full: Run picoclaw agent in Docker (interactive, full-featured)
docker-run-agent-full:
	docker compose -f docker/docker-compose.full.yml run --rm picoclaw-agent

## docker-clean: Clean Docker images and volumes
docker-clean:
	docker compose -f docker/docker-compose.yml down -v
	docker compose -f docker/docker-compose.full.yml down -v
	docker rmi picoclaw:latest picoclaw:full 2>/dev/null || true


## build-macos-app: Build PicoClaw macOS .app bundle (no terminal window)
build-macos-app:
	@echo "Building macOS .app bundle..."
	@if [ "$(UNAME_S)" != "Darwin" ]; then \
		echo "Error: This target is only available on macOS"; \
		exit 1; \
	fi
	@cd web && $(MAKE) build && cd ..
	@./scripts/build-macos-app.sh $(BINARY_NAME)-$(PLATFORM)-$(ARCH)
	@echo "macOS .app bundle created: $(BUILD_DIR)/PicoClaw.app"

## help: Show this help message
help:
	@echo "picoclaw Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sort | awk -F': ' '{printf "  %-16s %s\n", substr($$1, 4), $$2}'
	@echo ""
	@echo "Examples:"
	@echo "  make build              # Build for current platform"
	@echo "  make install            # Install to ~/.local/bin"
	@echo "  make uninstall          # Remove from /usr/local/bin"
	@echo "  make install-skills     # Install skills to workspace"
	@echo "  make docker-build       # Build minimal Docker image"
	@echo "  make docker-test        # Test MCP tools in Docker"
	@echo ""
	@echo "Environment Variables:"
	@echo "  INSTALL_PREFIX          # Installation prefix (default: ~/.local)"
	@echo "  WORKSPACE_DIR           # Workspace directory (default: ~/.picoclaw/workspace)"
	@echo "  VERSION                 # Version string (default: git describe)"
	@echo ""
	@echo "Current Configuration:"
	@echo "  Platform: $(PLATFORM)/$(ARCH)"
	@echo "  Binary: $(BINARY_PATH)"
	@echo "  Install Prefix: $(INSTALL_PREFIX)"
	@echo "  Workspace: $(WORKSPACE_DIR)"
