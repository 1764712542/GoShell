APP_NAME := goshell
MAIN := ./cmd/meatshell
VERSION := $(shell grep -oP 'version = "\K[^"]+' $(MAIN)/../meatshell 2>/dev/null || echo "0.1.0")
VERSION := 0.1.0

# 默认构建（当前平台）
.PHONY: build run clean package-all package-mac package-windows package-linux \
        release test test-coverage vet tidy

build:
	go build -o $(APP_NAME) $(MAIN)

run: build
	./$(APP_NAME)

# macOS 打包：生成 .app bundle
package-mac:
	@echo "Building macOS .app bundle..."
	go build -o $(APP_NAME) $(MAIN)
	@mkdir -p $(APP_NAME).app/Contents/{MacOS,Resources}
	cp $(APP_NAME) $(APP_NAME).app/Contents/MacOS/
	cp packaging/darwin/Info.plist $(APP_NAME).app/Contents/
	cp assets/icon.png $(APP_NAME).app/Contents/Resources/ 2>/dev/null || true
	@echo "Created $(APP_NAME).app"

# Windows 交叉编译（需要 mingw-w64）
package-windows:
	@echo "Cross-compiling for Windows (amd64)..."
	CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ \
	GOOS=windows GOARCH=amd64 go build -o $(APP_NAME).exe $(MAIN)
	@echo "Created $(APP_NAME).exe"

# Linux 打包
package-linux:
	@echo "Building for Linux (amd64)..."
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o $(APP_NAME)-linux-amd64 $(MAIN)
	@echo "Building for Linux (arm64)..."
	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -o $(APP_NAME)-linux-arm64 $(MAIN)
	@# 创建 tar.gz 包含 desktop entry
	tar -czf $(APP_NAME)-linux-amd64.tar.gz $(APP_NAME)-linux-amd64 packaging/linux/
	tar -czf $(APP_NAME)-linux-arm64.tar.gz $(APP_NAME)-linux-arm64 packaging/linux/
	@echo "Created Linux packages"

# 生成所有平台包（在 macOS 上）
package-all: package-mac package-windows package-linux
	@echo "All packages created."

# 使用 GoReleaser 发布（需要安装 goreleaser）
release:
	@echo "Running GoReleaser..."
	goreleaser --rm-dist

# test 运行全部测试并启用竞态检测器（-race）。
# -count=1 禁用测试结果缓存，确保每次都真正执行。
test:
	go test -race -count=1 ./...

# test-coverage 生成带竞态检测的覆盖率报告（HTML + 文本摘要）。
test-coverage:
	go test -race -count=1 -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	go tool cover -html=coverage.out -o coverage.html

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -f $(APP_NAME) $(APP_NAME).exe $(APP_NAME)-linux-*
	rm -rf $(APP_NAME).app
