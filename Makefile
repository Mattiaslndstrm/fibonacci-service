GOLANGCI_LINT_VERSION := 2.15.0
GOVULNCHECK_VERSION := v1.9.0

# Makes a changed version fetch the new release
GOLANGCI_LINT := bin/golangci-lint-$(GOLANGCI_LINT_VERSION)
GOLANGCI_LINT_URL := https://github.com/golangci/golangci-lint/releases/download/v$(GOLANGCI_LINT_VERSION)
GOLANGCI_LINT_NAME := golangci-lint-$(GOLANGCI_LINT_VERSION)-$(shell go env GOOS)-$(shell go env GOARCH)
GOLANGCI_LINT_SUMS := golangci-lint-$(GOLANGCI_LINT_VERSION)-checksums.txt

.PHONY: build test lint image

build:
	go build -o fibonacci-service .

test:
	go test -race ./...

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

image:
	docker build -t fibonacci-service .

$(GOLANGCI_LINT):
	mkdir -p bin
	cd bin && curl -sSfL -O $(GOLANGCI_LINT_URL)/$(GOLANGCI_LINT_NAME).tar.gz -O $(GOLANGCI_LINT_URL)/$(GOLANGCI_LINT_SUMS)
	cd bin && grep -qxF "$$(sha256sum $(GOLANGCI_LINT_NAME).tar.gz)" $(GOLANGCI_LINT_SUMS)
	tar -xzf bin/$(GOLANGCI_LINT_NAME).tar.gz -C bin --strip-components=1 $(GOLANGCI_LINT_NAME)/golangci-lint
	mv bin/golangci-lint $@
	rm bin/$(GOLANGCI_LINT_NAME).tar.gz bin/$(GOLANGCI_LINT_SUMS)
