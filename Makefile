# The waas CLI. The workflows run these targets too, so a check that passes
# here passes in CI.

VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)
# The platforms a release ships, as os/arch.
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

.PHONY: check fmt vet test build dist clean

# check is what a pull request must pass: formatted, vetted, tested.
check: vet test
	@test -z "$$(gofmt -l .)" || { echo "not formatted (run make fmt):"; gofmt -l .; exit 1; }

fmt:
	gofmt -w .

vet:
	go vet ./...

test:
	go test ./...

# build puts waas for this machine in bin/.
build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/waas ./cmd/waas

# dist builds the release binaries, waas-<os>-<arch>, and their checksums.
dist: clean
	@mkdir -p dist
	@for target in $(PLATFORMS); do \
		os=$${target%/*}; arch=$${target#*/}; \
		echo "waas-$$os-$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" \
			-o dist/waas-$$os-$$arch ./cmd/waas || exit 1; \
	done
	cd dist && sha256sum waas-* > checksums.txt

clean:
	rm -rf bin dist
