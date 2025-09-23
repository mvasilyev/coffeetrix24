# Coffeetrix24 Makefile
# Targets to setup Go, build, create a dedicated user, configure token, and run the app.

SHELL := /usr/bin/env bash
APP_NAME := coffeetrix24
APP_USER := coffeetrix
BIN_DIR := bin
BIN := $(BIN_DIR)/bot
PKG := ./cmd/bot
DB_PATH := ./data/coffeetrix.db
LOG_DIR := logs
RUN_DIR := run
PID_FILE := $(RUN_DIR)/$(APP_NAME).pid

UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)

.PHONY: help check-go install-go ensure-go deps build create-user configure run start run-detached start-detached stop status test-run test-run-detached setup setup-run clean build-linux-amd64-docker build-linux-386-docker build-linux-amd64-zig

# Minimal and desired Go versions
GO_MIN_VER := 1.18
GO_DESIRED_VER := 1.22.7

# Resolve a usable Go binary (prefer /usr/local/go/bin/go if present)
GO := $(shell if [ -x /usr/local/go/bin/go ]; then echo /usr/local/go/bin/go; elif command -v go >/dev/null 2>&1; then command -v go; else echo go; fi)
DOCKER_IMAGE ?= golang:1.18-bullseye
DOCKER_PLATFORM ?= linux/amd64

check-go-version:
	@if command -v $(GO) >/dev/null 2>&1; then \
		VER=$$($(GO) env GOVERSION | sed 's/go//'); \
		REQ=$(GO_MIN_VER); \
		awk -v v1=$$VER -v v2=$$REQ 'BEGIN { split(v1,a,"."); split(v2,b,"."); if (a[1]<b[1] || (a[1]==b[1] && a[2]<b[2])) exit 1; else exit 0; }'; \
		if [ $$? -ne 0 ]; then echo "Go $$VER < $(GO_MIN_VER)."; exit 1; fi; \
	else \
		echo "Go not found"; exit 1; \
	fi

help:
	@echo "Usage: make <target>"
	@echo "Targets:"
	@echo "  check-go     - Check if Go is installed"
	@echo "  install-go   - Install Go via Homebrew if not present (macOS)"
	@echo "  deps         - Download/verify dependencies (go mod tidy)"
	@echo "  build        - Build $(BIN) for host architecture"
	@echo "  create-user  - Create dedicated macOS user '$(APP_USER)' (requires sudo)"
	@echo "  configure    - Ask for Telegram bot token and write .env"
	@echo "  run          - Run app as '$(APP_USER)' using .env"
	@echo "  start        - Same as run"
	@echo "  test-run     - Run in test mode (immediate invite, 1 min window)"
	@echo "  setup-run    - Install Go if missing, build, create user, configure, and run"
	@echo "  clean        - Remove built binaries"
	@echo "  build-linux-amd64-docker - Cross-compile linux/amd64 binary via Docker (CGO enabled)"
	@echo "  build-linux-386-docker   - Cross-compile linux/386  binary via Docker (CGO enabled)"
	@echo "  build-linux-amd64-zig    - Cross-compile linux/amd64 locally using zig cc (if available)"

check-go:
	@if command -v go >/dev/null 2>&1; then \
		echo "Go found: $$(go version)"; \
	else \
		echo "Go not found"; exit 1; \
	fi

install-go:
	@if command -v go >/dev/null 2>&1; then \
		echo "Go already installed: $$(go version)"; \
	else \
		if [[ "$(UNAME_S)" == "Darwin" ]]; then \
			if command -v brew >/dev/null 2>&1; then \
				echo "Installing Go via Homebrew..."; \
				brew update && brew install go; \
			else \
				echo "Homebrew not found. Install Go from https://go.dev/dl/ or install Homebrew: https://brew.sh"; exit 1; \
			fi; \
		elif [[ "$(UNAME_S)" == "Linux" ]]; then \
			if command -v apt-get >/dev/null 2>&1; then sudo apt-get update && sudo apt-get install -y golang-go || true; \
			elif command -v dnf >/dev/null 2>&1; then sudo dnf install -y golang || true; \
			elif command -v yum >/dev/null 2>&1; then sudo yum install -y golang || true; \
			elif command -v pacman >/dev/null 2>&1; then sudo pacman -Sy --noconfirm go || true; \
			elif command -v zypper >/dev/null 2>&1; then sudo zypper install -y go || true; \
			else echo "Unsupported Linux package manager. Install Go from https://go.dev/dl/"; exit 1; \
			fi; \
		else \
			echo "Unsupported OS: $(UNAME_S). Install Go from https://go.dev/dl/"; exit 1; \
		fi; \
	fi

# Install specific Go version if current is missing or too old
ensure-go:
	@# Check current Go and compare with minimal requirement
	@if command -v $(GO) >/dev/null 2>&1; then \
		CUR=$$($(GO) env GOVERSION | sed 's/go//'); \
		REQ=$(GO_MIN_VER); \
		awk -v v1=$$CUR -v v2=$$REQ 'BEGIN { split(v1,a,"."); split(v2,b,"."); if (a[1]<b[1] || (a[1]==b[1] && a[2]<b[2])) exit 1; else exit 0; }'; \
		if [ $$? -eq 0 ]; then echo "Go OK: $$($(GO) version)"; exit 0; fi; \
	fi; \
	# Install or upgrade Go
	if [ "$(UNAME_S)" = "Darwin" ]; then \
		if command -v brew >/dev/null 2>&1; then echo "Installing/Upgrading Go via Homebrew..."; brew update && (brew install go || brew upgrade go) || true; else echo "Install Go from https://go.dev/dl/ or install Homebrew: https://brew.sh"; exit 1; fi; \
	elif [ "$(UNAME_S)" = "Linux" ]; then \
		ARCH=$$(uname -m); \
		if [ "$$ARCH" = "x86_64" ]; then GO_DL_ARCH=amd64; \
		elif [ "$$ARCH" = "aarch64" ] || [ "$$ARCH" = "arm64" ]; then GO_DL_ARCH=arm64; \
		elif [ "$$ARCH" = "armv7l" ]; then GO_DL_ARCH=armv6l; \
		else echo "Unsupported ARCH: $$ARCH"; exit 1; fi; \
		URL=https://go.dev/dl/go$(GO_DESIRED_VER).linux-$$GO_DL_ARCH.tar.gz; \
		echo "Installing Go $(GO_DESIRED_VER) from $$URL"; \
		sudo rm -rf /usr/local/go; \
		curl -fsSL $$URL | sudo tar -C /usr/local -xzf -; \
	else \
		echo "Unsupported OS: $(UNAME_S)"; exit 1; \
	fi; \
	echo "Go installed: $$(/usr/local/go/bin/go version)"

$(BIN):
	@mkdir -p $(BIN_DIR) $(dir $(DB_PATH)) $(LOG_DIR) $(RUN_DIR)
	@echo "Building for host: GOOS=$$($(GO) env GOOS) GOARCH=$$($(GO) env GOARCH) (CGO enabled)"
	@if [ "$(UNAME_S)" = "Linux" ]; then \
		if ! command -v gcc >/dev/null 2>&1; then \
			echo "Installing build tools (requires sudo)..."; \
			if command -v apt-get >/dev/null 2>&1; then sudo apt-get update && sudo apt-get install -y build-essential; \
			elif command -v dnf >/dev/null 2>&1; then sudo dnf groupinstall -y "Development Tools" || true; \
			elif command -v yum >/dev/null 2>&1; then sudo yum groupinstall -y "Development Tools" || true; \
			elif command -v pacman >/dev/null 2>&1; then sudo pacman -Sy --noconfirm base-devel; \
			elif command -v zypper >/dev/null 2>&1; then sudo zypper install -y gcc make; \
			fi; \
		fi; \
	fi
	@CGO_ENABLED=1 $(GO) build -o $(BIN) $(PKG)

build: ensure-go deps $(BIN)

deps:
	@$(GO) mod tidy

create-user:
	@if id -u $(APP_USER) >/dev/null 2>&1; then \
		echo "User '$(APP_USER)' already exists"; \
	else \
		echo "Creating user '$(APP_USER)' (requires sudo)..."; \
		case "$(UNAME_S)" in \
			Darwin) \
				PASS=$$(LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 16); \
				if command -v sysadminctl >/dev/null 2>&1; then \
					sudo sysadminctl -addUser $(APP_USER) -fullName "Coffeetrix Bot" -password "$$PASS" -home /var/empty -shell /usr/bin/false || true; \
				else \
					echo "sysadminctl not found. Attempting dscl..."; \
					sudo dscl . -create /Users/$(APP_USER); \
					sudo dscl . -create /Users/$(APP_USER) UserShell /usr/bin/false; \
					sudo dscl . -create /Users/$(APP_USER) RealName "Coffeetrix Bot"; \
					sudo dscl . -create /Users/$(APP_USER) NFSHomeDirectory /var/empty; \
					sudo dscl . -passwd /Users/$(APP_USER) "$$PASS"; \
				fi; \
				unset PASS ;; \
			Linux) \
				NOLOGIN=$$(command -v nologin 2>/dev/null || echo /usr/sbin/nologin); \
				sudo useradd -r -s $$NOLOGIN -M -U $(APP_USER) 2>/dev/null || echo "Useradd may have reported already exists" ;; \
			*) echo "Unsupported OS: $(UNAME_S)" ;; \
		esac; \
	fi
	@mkdir -p $(dir $(DB_PATH)) $(LOG_DIR) $(RUN_DIR)
	@sudo chown -R $(APP_USER) $(dir $(DB_PATH)) $(LOG_DIR) $(RUN_DIR) || true

configure:
	@echo "Configuring .env..."
	@if [ -f .env ]; then \
		echo ".env already exists. It will be overwritten."; \
	fi
	@printf "Введите TELEGRAM_BOT_TOKEN: "; \
	read TOKEN; \
	if [ -z "$$TOKEN" ]; then echo "Токен не задан"; exit 1; fi; \
	echo "TELEGRAM_BOT_TOKEN=$$TOKEN" > .env.tmp; \
	echo "DATABASE_PATH=$(DB_PATH)" >> .env.tmp; \
	mv .env.tmp .env; \
	echo ".env updated.";
	@sudo chown $(APP_USER) .env || true
	@sudo chmod 640 .env || true

run:
	@echo "Starting app as '$(APP_USER)'..."
	@if [ ! -f $(BIN) ]; then echo "Binary not found. Run 'make build' first."; exit 1; fi
	@if [ ! -f .env ]; then echo ".env not found. Run 'make configure' first."; exit 1; fi
	@sudo -u $(APP_USER) env TELEGRAM_BOT_TOKEN="$$(grep -E '^TELEGRAM_BOT_TOKEN=' .env | sed 's/.*=//')" DATABASE_PATH="$(DB_PATH)" $(BIN)

start: run

run-detached:
	@echo "Starting app (detached) as '$(APP_USER)'..."
	@if [ ! -f $(BIN) ]; then echo "Binary not found. Run 'make build' first."; exit 1; fi
	@if [ ! -f .env ]; then echo ".env not found. Run 'make configure' first."; exit 1; fi
	@mkdir -p $(LOG_DIR) $(RUN_DIR)
	@sudo -u $(APP_USER) sh -c 'nohup env TELEGRAM_BOT_TOKEN="$$(grep -E "^TELEGRAM_BOT_TOKEN=" .env | sed "s/.*=//")" DATABASE_PATH="$(DB_PATH)" $(BIN) $(RUN_FLAGS) >> $(LOG_DIR)/app.log 2>&1 & echo $$! > $(PID_FILE)'
	@echo "Started. PID stored in $(PID_FILE). Logs: $(LOG_DIR)/app.log"

start-detached: run-detached

test-run:
	@echo "Starting app in TEST mode as '$(APP_USER)'..."
	@if [ ! -f $(BIN) ]; then echo "Binary not found. Run 'make build' first."; exit 1; fi
	@if [ ! -f .env ]; then echo ".env not found. Run 'make configure' first."; exit 1; fi
	@sudo -u $(APP_USER) env TELEGRAM_BOT_TOKEN="$$(grep -E '^TELEGRAM_BOT_TOKEN=' .env | sed 's/.*=//')" DATABASE_PATH="$(DB_PATH)" $(BIN) --test

test-run-detached:
	@echo "Starting app in TEST mode (detached) as '$(APP_USER)'..."
	@if [ ! -f $(BIN) ]; then echo "Binary not found. Run 'make build' first."; exit 1; fi
	@if [ ! -f .env ]; then echo ".env not found. Run 'make configure' first."; exit 1; fi
	@mkdir -p $(LOG_DIR) $(RUN_DIR)
	@sudo -u $(APP_USER) sh -c 'nohup env TELEGRAM_BOT_TOKEN="$$(grep -E "^TELEGRAM_BOT_TOKEN=" .env | sed "s/.*=//")" DATABASE_PATH="$(DB_PATH)" $(BIN) --test >> $(LOG_DIR)/app.log 2>&1 & echo $$! > $(PID_FILE)'
	@echo "Started (test). PID stored in $(PID_FILE). Logs: $(LOG_DIR)/app.log"

stop:
	@if [ -f $(PID_FILE) ]; then \
		PID=$$(cat $(PID_FILE)); \
		if kill -0 $$PID >/dev/null 2>&1; then \
			echo "Stopping PID $$PID..."; \
			kill $$PID || true; \
			sleep 1; \
			if kill -0 $$PID >/dev/null 2>&1; then echo "Force killing PID $$PID"; kill -9 $$PID || true; fi; \
		else \
			echo "Process $$PID not running"; \
		fi; \
		rm -f $(PID_FILE); \
	else \
		echo "No PID file at $(PID_FILE)."; \
	fi

status:
	@if [ -f $(PID_FILE) ]; then \
		PID=$$(cat $(PID_FILE)); \
		if kill -0 $$PID >/dev/null 2>&1; then echo "Running (PID $$PID)"; else echo "Not running (stale PID $$PID)"; fi; \
	else \
		echo "Not running (no PID file)."; \
	fi

setup: ensure-go deps build create-user configure

setup-run: ensure-go deps build create-user configure start-detached

clean:
	@rm -f $(BIN)

# --- Cross-compile helpers ---
# Build inside a Linux container so CGO (sqlite3) links against glibc.
build-linux-amd64-docker:
	@mkdir -p $(BIN_DIR)
	@docker run --rm --platform=$(DOCKER_PLATFORM) -v "$$PWD":/src -w /src $(DOCKER_IMAGE) bash -lc \
		"set -euo pipefail; apt-get update >/dev/null; apt-get install -y -qq build-essential >/dev/null; export PATH=/usr/local/go/bin:\$$PATH; CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -v -o $(BIN_DIR)/bot-linux-amd64 $(PKG)"
	@echo "Built $(BIN_DIR)/bot-linux-amd64"

build-linux-386-docker:
	@mkdir -p $(BIN_DIR)
	@docker run --rm --platform=$(DOCKER_PLATFORM) -v "$$PWD":/src -w /src $(DOCKER_IMAGE) bash -lc \
		"set -euo pipefail; apt-get update >/dev/null; apt-get install -y -qq build-essential gcc-multilib >/dev/null; export PATH=/usr/local/go/bin:\$$PATH; CGO_ENABLED=1 GOOS=linux GOARCH=386 go build -v -o $(BIN_DIR)/bot-linux-386 $(PKG)"
	@echo "Built $(BIN_DIR)/bot-linux-386"

# Optional: local cross-compile using zig cc (no Docker). Requires 'zig' installed.
build-linux-amd64-zig:
	@command -v zig >/dev/null 2>&1 || { echo "zig not found. Install zig or use build-linux-amd64-docker"; exit 1; }
	@mkdir -p $(BIN_DIR)
	@env CC="zig cc -target x86_64-linux-gnu" CXX="zig c++ -target x86_64-linux-gnu" CGO_ENABLED=1 GOOS=linux GOARCH=amd64 $(GO) build -o $(BIN_DIR)/bot-linux-amd64 $(PKG)
	@echo "Built $(BIN_DIR)/bot-linux-amd64 (zig)"
