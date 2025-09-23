# Coffeetrix24 Makefile
# Targets to setup Go, build, create a dedicated user, configure token, and run the app.

SHELL := /bin/sh
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

.PHONY: help check-go install-go deps build create-user configure run start run-detached start-detached stop status test-run test-run-detached setup setup-run clean

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
		case "$(UNAME_S)" in \
			Darwin) \
				if command -v brew >/dev/null 2>&1; then \
					echo "Installing Go via Homebrew..."; \
					brew update && brew install go; \
				else \
					echo "Homebrew not found. Install Go from https://go.dev/dl/ or install Homebrew: https://brew.sh"; exit 1; \
				fi ;; \
			Linux) \
				if command -v apt-get >/dev/null 2>&1; then \
					sudo apt-get update && sudo apt-get install -y golang-go; \
				elif command -v dnf >/dev/null 2>&1; then \
					sudo dnf install -y golang; \
				elif command -v yum >/dev/null 2>&1; then \
					sudo yum install -y golang; \
				elif command -v pacman >/dev/null 2>&1; then \
					sudo pacman -Sy --noconfirm go; \
				elif command -v zypper >/dev/null 2>&1; then \
					sudo zypper install -y go; \
				else \
					echo "Unsupported package manager. Install Go from https://go.dev/dl/"; exit 1; \
				fi ;; \
			*) echo "Unsupported OS: $(UNAME_S). Install Go from https://go.dev/dl/"; exit 1 ;; \
		esac; \
	fi

$(BIN):
	@mkdir -p $(BIN_DIR) $(dir $(DB_PATH)) $(LOG_DIR) $(RUN_DIR)
	@echo "Building for host: GOOS=$$(go env GOOS) GOARCH=$$(go env GOARCH)"
	@CGO_ENABLED=0 go build -o $(BIN) $(PKG)

build: check-go deps $(BIN)

deps:
	@go mod tidy

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

setup: install-go deps build create-user configure

setup-run: install-go deps build create-user configure start-detached

clean:
	@rm -f $(BIN)
