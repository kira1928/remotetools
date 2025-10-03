# Makefile for Remote Tools CLI

# 应用程序名称
APP_NAME := remotetools

# 版本信息
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GO_VERSION := $(shell go version | awk '{print $$3}')

# 构建目录
BUILD_DIR := build
DIST_DIR := dist

# Go 构建标志
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GoVersion=$(GO_VERSION)"
DEBUG_FLAGS := -gcflags="all=-N -l"
RELEASE_FLAGS := -ldflags "-s -w -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GoVersion=$(GO_VERSION)"

# 支持的操作系统和架构
PLATFORMS := \
	darwin/amd64 \
	darwin/arm64 \
	linux/amd64 \
	linux/arm64 \
	linux/386 \
	windows/amd64 \
	windows/386 \
	windows/arm64

# 当前平台
CURRENT_OS := $(shell go env GOOS)
CURRENT_ARCH := $(shell go env GOARCH)

.PHONY: all clean debug release build-all help install test

# 默认目标：构建当前平台
all: build

# 显示帮助信息
help:
	@echo "Remote Tools 构建系统"
	@echo ""
	@echo "可用的目标:"
	@echo "  make build        - 构建当前平台的可执行文件"
	@echo "  make debug        - 构建当前平台的 debug 版本"
	@echo "  make release      - 构建当前平台的 release 版本"
	@echo "  make build-all    - 构建所有平台的可执行文件"
	@echo "  make install      - 安装到 GOPATH/bin"
	@echo "  make test         - 运行测试"
	@echo "  make clean        - 清理构建文件"
	@echo "  make help         - 显示此帮助信息"
	@echo ""
	@echo "环境变量:"
	@echo "  GOOS              - 目标操作系统 (darwin, linux, windows)"
	@echo "  GOARCH            - 目标架构 (amd64, arm64, 386)"
	@echo ""
	@echo "示例:"
	@echo "  make build                    # 构建当前平台"
	@echo "  make release                  # 构建当前平台 release 版"
	@echo "  GOOS=linux GOARCH=amd64 make build  # 交叉编译到 Linux"
	@echo "  make build-all                # 构建所有平台"

# 构建当前平台（默认）
build:
	@echo "构建 $(CURRENT_OS)/$(CURRENT_ARCH)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)$(if $(filter windows,$(CURRENT_OS)),.exe,) ./cmd/main.go
	@echo "构建完成: $(BUILD_DIR)/$(APP_NAME)$(if $(filter windows,$(CURRENT_OS)),.exe,)"

# 构建 debug 版本
debug:
	@echo "构建 debug 版本 ($(CURRENT_OS)/$(CURRENT_ARCH))..."
	@mkdir -p $(BUILD_DIR)
	go build $(DEBUG_FLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-debug$(if $(filter windows,$(CURRENT_OS)),.exe,) ./cmd/main.go
	@echo "构建完成: $(BUILD_DIR)/$(APP_NAME)-debug$(if $(filter windows,$(CURRENT_OS)),.exe,)"

# 构建 release 版本
release:
	@echo "构建 release 版本 ($(CURRENT_OS)/$(CURRENT_ARCH))..."
	@mkdir -p $(BUILD_DIR)
	go build $(RELEASE_FLAGS) -o $(BUILD_DIR)/$(APP_NAME)$(if $(filter windows,$(CURRENT_OS)),.exe,) ./cmd/main.go
	@echo "构建完成: $(BUILD_DIR)/$(APP_NAME)$(if $(filter windows,$(CURRENT_OS)),.exe,)"
	@if [ "$(CURRENT_OS)" != "windows" ]; then \
		strip $(BUILD_DIR)/$(APP_NAME) 2>/dev/null || true; \
	fi

# 构建所有平台
build-all: clean
	@echo "开始构建所有平台..."
	@mkdir -p $(DIST_DIR)
	@$(foreach platform,$(PLATFORMS), \
		$(eval OS := $(word 1,$(subst /, ,$(platform)))) \
		$(eval ARCH := $(word 2,$(subst /, ,$(platform)))) \
		echo "构建 $(OS)/$(ARCH)..."; \
		GOOS=$(OS) GOARCH=$(ARCH) go build $(RELEASE_FLAGS) \
			-o $(DIST_DIR)/$(APP_NAME)-$(OS)-$(ARCH)$(if $(filter windows,$(OS)),.exe,) \
			./cmd/main.go || exit 1; \
		if [ "$(OS)" != "windows" ]; then \
			strip $(DIST_DIR)/$(APP_NAME)-$(OS)-$(ARCH) 2>/dev/null || true; \
		fi; \
	)
	@echo ""
	@echo "所有平台构建完成！文件位于 $(DIST_DIR)/ 目录"
	@ls -lh $(DIST_DIR)/

# 安装到 GOPATH/bin
install:
	@echo "安装到 GOPATH/bin..."
	go install $(LDFLAGS) ./cmd/main.go
	@echo "安装完成"

# 运行测试
test:
	@echo "运行测试..."
	go test -v ./...

# 清理构建文件
clean:
	@echo "清理构建文件..."
	@rm -rf $(BUILD_DIR) $(DIST_DIR)
	@echo "清理完成"

# 运行构建的程序
run: build
	@./$(BUILD_DIR)/$(APP_NAME)$(if $(filter windows,$(CURRENT_OS)),.exe,)
