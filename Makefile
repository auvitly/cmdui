GO ?= go

ifeq ($(OS),Windows_NT)
BINARY := cmdui.exe
RUN := .\$(BINARY)
STOP_COMMAND := taskkill /IM $(BINARY) /T /F
else
BINARY := cmdui
RUN := ./$(BINARY)
STOP_COMMAND := pkill -x $(BINARY) || true
endif

.PHONY: all build run stop restart test

all: build

build:
	$(GO) build -o $(BINARY) ./cmd/server

run: build
	$(RUN)

stop:
	-$(STOP_COMMAND)

restart: stop build
	$(RUN)

test:
	$(GO) test ./...