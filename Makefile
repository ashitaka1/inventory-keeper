
GO_BUILD_ENV :=
GO_BUILD_FLAGS :=
MODULE_BINARY := bin/inventory-keeper

ifeq ($(VIAM_TARGET_OS), windows)
	GO_BUILD_ENV += GOOS=windows GOARCH=amd64
	GO_BUILD_FLAGS := -tags no_cgo
	MODULE_BINARY = bin/inventory-keeper.exe
endif

$(MODULE_BINARY): Makefile go.mod *.go cmd/module/*.go 
	GOOS=$(VIAM_BUILD_OS) GOARCH=$(VIAM_BUILD_ARCH) $(GO_BUILD_ENV) go build $(GO_BUILD_FLAGS) -o $(MODULE_BINARY) cmd/module/main.go

lint:
	gofmt -s -w .

update:
	go get go.viam.com/rdk@latest
	go mod tidy

test:
	go test ./...

module.tar.gz: meta.json $(MODULE_BINARY)
ifneq ($(VIAM_TARGET_OS), windows)
	strip $(MODULE_BINARY)
endif
	tar czf $@ meta.json $(MODULE_BINARY)

module: test module.tar.gz

all: test module.tar.gz

setup:
	go mod tidy

test-qr:
	@echo "Generating test QR codes from testdata/items.json..."
	@mkdir -p testdata/qr
	@jq -c '.[]' testdata/items.json | while read item; do \
		item_id=$$(echo $$item | jq -r '.item_id'); \
		echo "  Generating QR code for $$item_id..."; \
		echo $$item | qrencode -s 10 -o testdata/qr/qr-$$item_id.png; \
	done
	@echo "Done! QR codes saved to testdata/qr/"
	@ls -1 testdata/qr/ | wc -l | xargs echo "Generated" | xargs -I {} echo "{} QR codes"
