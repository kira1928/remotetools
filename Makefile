## 轻量 Makefile：仅作为入口调用 build.go，实现跨平台构建

.PHONY: all build dev release build-all install test clean run help build-web

BUILD_TAGS := buildtool
GO_RUN := go run -tags $(BUILD_TAGS) ./build.go

# 如果用户在命令行设置了 GOOS/GOARCH，则透传给 build.go（可选）
PLATFORM_FLAGS := $(if $(GOOS), -os $(GOOS),) $(if $(GOARCH), -arch $(GOARCH),)

# 默认目标：构建当前平台
all: build

help:
	$(GO_RUN) help

build:
	$(GO_RUN) build $(PLATFORM_FLAGS)

dev:
	$(GO_RUN) dev $(PLATFORM_FLAGS)

release:
	$(GO_RUN) release $(PLATFORM_FLAGS)

build-all: clean
	$(GO_RUN) build-all

install:
	$(GO_RUN) install

test:
	$(GO_RUN) test

clean:
	$(GO_RUN) clean

run:
	$(GO_RUN) run


# 构建前端 Web UI
build-web:
	cd web && npm install && npm run build
