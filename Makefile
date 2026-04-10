# 專案變數
BINARY_NAME=SpaceWatcher-go
MAIN_PATH=./cmd
GO_VERSION_FILE=go.mod

# 預設執行 build
.PHONY: all
all: tidy build

## build: 編譯當前平台的二進位檔
.PHONY: build
build:
	@echo "==> Building $(BINARY_NAME)..."
	@go build -ldflags="-s -w" -o $(BINARY_NAME) $(MAIN_PATH)

## test: 執行單元測試
.PHONY: test
test:
	@echo "==> Running tests..."
	@go test -v ./...

## lint: 執行 Linter 檢查
.PHONY: lint
lint:
	@echo "==> Running golangci-lint..."
	@golangci-lint run

## tidy: 整理 Go modules
.PHONY: tidy
tidy:
	@echo "==> Tidying modules..."
	@go mod tidy

## clean: 清除編譯產物
.PHONY: clean
clean:
	@echo "==> Cleaning up..."
	@rm -f $(BINARY_NAME)
	@rm -rf dist/

## snapshot: 測試 GoReleaser 設定 (不發佈，僅本地編譯)
.PHONY: snapshot
snapshot:
	@echo "==> Testing GoReleaser snapshot..."
	@goreleaser release --snapshot --clean

## help: 顯示指令幫助
.PHONY: help
help:
	@echo "Usage:"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^ / /'
