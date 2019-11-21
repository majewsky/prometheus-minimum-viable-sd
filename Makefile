CMD=prometheus-minimum-viable-sd
PKG=github.com/majewsky/$(CMD)
PREFIX=/usr

all: build/$(CMD)

# NOTE: This repo uses Go modules, and uses a synthetic GOPATH at
# $(CURDIR)/.gopath that is only used for the build cache. $GOPATH/src/ is
# empty.
GO            = GOPATH=$(CURDIR)/.gopath GOBIN=$(CURDIR)/build go
GO_BUILDFLAGS =
GO_LDFLAGS    = -s -w

build/$(CMD): FORCE
	$(GO) install $(GO_BUILDFLAGS) -ldflags '$(GO_LDFLAGS)' '$(PKG)'

install: FORCE all
	install -D -m 0755 "build/$(CMD)" "$(DESTDIR)$(PREFIX)/bin/$(CMD)"
	install -D -m 0644 README.md "$(DESTDIR)$(PREFIX)/share/doc/$(CMD)/README.md"

vendor: FORCE
	$(GO) mod tidy
	$(GO) mod vendor

.PHONY: FORCE
