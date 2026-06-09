BINARY    := searchcore
BUILD_DIR := bin
CMD_DIR   := cmd/server

# C++ index library
CPP_DIR    := internal/index/cpp
CPP_BUILD  := $(CPP_DIR)/build
LIB_DIR    := lib
LIB        := $(LIB_DIR)/libsearchindex.a
STEM_DIR   := $(CPP_DIR)/vendor/libstemmer

CXX       ?= g++
CC        ?= gcc
CXXFLAGS  := -std=c++17 -O2 -fPIC \
             -I$(CPP_DIR)/include \
             -I$(STEM_DIR)/include \
             -I$(STEM_DIR)/runtime
CFLAGS_C  := -O2 \
             -I$(STEM_DIR)/include \
             -I$(STEM_DIR)/runtime

CPP_SRCS  := $(wildcard $(CPP_DIR)/src/*.cpp)
C_SRCS    := $(STEM_DIR)/runtime/api.c \
             $(STEM_DIR)/runtime/utilities.c \
             $(STEM_DIR)/src_c/stem_UTF_8_english.c \
             $(STEM_DIR)/libstemmer/libstemmer.c

CPP_OBJS  := $(patsubst $(CPP_DIR)/src/%.cpp,$(CPP_BUILD)/%.o,$(CPP_SRCS))
C_OBJS    := $(CPP_BUILD)/stem_api.o \
             $(CPP_BUILD)/stem_utilities.o \
             $(CPP_BUILD)/stem_stem_UTF_8_english.o \
             $(CPP_BUILD)/stem_libstemmer.o

.PHONY: build test test-short proto seed docker-up docker-down lint clean run cpp-lib

# ---- C++ static library ----

cpp-lib: $(LIB)

$(LIB): $(CPP_OBJS) $(C_OBJS)
	mkdir -p $(LIB_DIR)
	ar rcs $@ $^

$(CPP_BUILD)/%.o: $(CPP_DIR)/src/%.cpp
	mkdir -p $(CPP_BUILD)
	$(CXX) $(CXXFLAGS) -c $< -o $@

$(CPP_BUILD)/stem_api.o: $(STEM_DIR)/runtime/api.c
	mkdir -p $(CPP_BUILD)
	$(CC) $(CFLAGS_C) -c $< -o $@

$(CPP_BUILD)/stem_utilities.o: $(STEM_DIR)/runtime/utilities.c
	mkdir -p $(CPP_BUILD)
	$(CC) $(CFLAGS_C) -c $< -o $@

$(CPP_BUILD)/stem_stem_UTF_8_english.o: $(STEM_DIR)/src_c/stem_UTF_8_english.c
	mkdir -p $(CPP_BUILD)
	$(CC) $(CFLAGS_C) -I$(STEM_DIR)/src_c -c $< -o $@

$(CPP_BUILD)/stem_libstemmer.o: $(STEM_DIR)/libstemmer/libstemmer.c
	mkdir -p $(CPP_BUILD)
	$(CC) $(CFLAGS_C) -c $< -o $@

# ---- Go targets ----

build: cpp-lib
	go build -o $(BUILD_DIR)/$(BINARY) ./$(CMD_DIR)

test: cpp-lib
	go test -v -race ./...

test-short: cpp-lib
	go test -short ./...

proto:
	protoc --go_out=. --go-grpc_out=. \
		--go_opt=paths=source_relative \
		--go-grpc_opt=paths=source_relative \
		proto/searchcore.proto

seed:
	go run ./scripts/seed.go

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

docker-reset:
	docker-compose down -v

run: build
	./$(BUILD_DIR)/$(BINARY)

lint:
	golangci-lint run ./...

clean:
	rm -rf $(BUILD_DIR) $(LIB_DIR) $(CPP_BUILD)
