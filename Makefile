BINARY  := nvme_logpage_exporter
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
REVISION?= $(shell git rev-parse --verify HEAD 2>/dev/null || echo unknown)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
FUZZTARGET ?= FuzzParseSmart
FUZZTIME   ?= 30s

VPKG    := github.com/prometheus/common/version
LDFLAGS := -s -w \
	-X $(VPKG).Version=$(VERSION) \
	-X $(VPKG).Revision=$(REVISION) \
	-X $(VPKG).BuildDate=$(DATE)

DIST_DIR := dist

.PHONY: build test lint vet vulncheck fuzz dist clean docker smoke install

build:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o bin/$(BINARY) ./cmd/$(BINARY)

test:
	go test -race ./...

# GOOS=linux or the unused linter cannot see ioctl_linux.go and calls its
# dependencies dead code.
lint:
	GOOS=linux golangci-lint run

vet:
	go vet ./...

vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

fuzz:
	go test ./internal/nvme/logpage/ -run=NONE -fuzz='^$(FUZZTARGET)$$' -fuzztime=$(FUZZTIME)

dist:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST_DIR)/$(BINARY)-linux-amd64 ./cmd/$(BINARY)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST_DIR)/$(BINARY)-linux-arm64 ./cmd/$(BINARY)

clean:
	rm -rf bin/ $(DIST_DIR)/

docker: dist
	docker build --platform linux/amd64 -t $(BINARY):$(VERSION) .

# `timeout`, not backgrounding plus `kill %1`: make's shell has no job
# control, and killing sudo does not reach the child. Nothing may be left
# running with CAP_SYS_ADMIN.
smoke: build
	@echo "Requires a host with NVMe and CAP_SYS_ADMIN"
	sudo ./bin/$(BINARY) dump --out /tmp/nvme-smoke
	sudo timeout 10 ./bin/$(BINARY) --web.listen-address=:9683 & \
	for i in $$(seq 1 20); do \
		curl -sf localhost:9683/metrics >/dev/null 2>&1 && break ; \
		sleep 0.5 ; \
	done ; \
	count=$$(curl -s localhost:9683/metrics | grep -c '^nvme_logpage_') ; \
	echo "$$count nvme_logpage_ series" ; \
	wait ; \
	test "$$count" -gt 0

# Does not create the nvme_logpage user/group or enable the service.
install: build
	install -m 0755 bin/$(BINARY) /usr/local/bin/$(BINARY)
	install -m 0644 packaging/systemd/nvme_logpage_exporter.service /etc/systemd/system/
	install -m 0644 packaging/udev/99-nvme-logpage-exporter.rules /etc/udev/rules.d/
	udevadm control --reload
	udevadm trigger
