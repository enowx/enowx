HOST_GO ?= go
GO ?= $(HOST_GO) run ./scripts/terminalpatch --
BIN := bin
# Where `make dev` keeps its daemon, sessions, projects and hosts, apart
# from the installed app's ~/.enowx-app.
DEV_HOME ?= $(HOME)/.enowx-dev
# The app is a GUI program on Windows: built as a console one, it opens a
# console window of its own, and closing that window closes the app. The
# daemon stays a console program, its output going to enowxd.log.
APP_LDFLAGS :=
ifeq ($(OS),Windows_NT)
APP_LDFLAGS := -ldflags "-H=windowsgui"
endif

.PHONY: build daemons dev test lint fmt run-app run-daemon clean package

build: daemons
	$(GO) build -o $(BIN)/ ./cmd/enowxd
	$(GO) build -o $(BIN)/ $(APP_LDFLAGS) ./cmd/enowx

# Linux builds of enowxd that the app installs on servers.
daemons:
	HOST_GO=$(HOST_GO) scripts/daemons.sh

test:
	$(GO) test ./...

# Every platform's installers, built here into dist/<version>, not published.
package:
	scripts/package.sh

# The app with hot reload: Go changes rebuild and relaunch it, and the
# relaunched app replaces the daemon an earlier build started. The Linux
# daemons are built first: the app embeds them to install on servers.
dev: daemons
	ENOWX_DEV=1 ENOWX_HOME=$(DEV_HOME) $(GO) tool mygo dev cmd/enowx

lint:
	$(GO) vet ./...
	$(GO) run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...

fmt:
	gofmt -w .

run-app:
	$(GO) run ./cmd/enowx

run-daemon:
	$(GO) run ./cmd/enowxd serve

clean:
	rm -rf $(BIN)
