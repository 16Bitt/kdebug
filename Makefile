SOURCES 			= $(wildcard *.go)
OUTDIR				= build
BUILD_SUFFIX	= noarch
TARGET_BASE		= kdebug
TARGET_PREFIX	= $(OUTDIR)/$(TARGET_BASE)
TARGETS				= $(TARGET_PREFIX)-darwin-x86_64 \
								$(TARGET_PREFIX)-darwin-arm64 \
								$(TARGET_PREFIX)-linux-x86_64 \
								$(TARGET_PREFIX)-linux-arm64 \
								$(TARGET_PREFIX)-noarch

all: $(TARGETS)

clean:
	-rm -r $(OUTDIR)

$(OUTDIR):
	mkdir -p build

$(TARGETS): $(OUTDIR) $(SOURCES)
	$(OPTS) go build -o $(OUTDIR)/kdebug-$(BUILD_SUFFIX) $(SOURCES)

$(TARGET_PREFIX)-darwin-x86_64: OPTS := GOOS=darwin GOARCH=amd64
$(TARGET_PREFIX)-darwin-x86_64: BUILD_SUFFIX := darwin-x86_64

$(TARGET_PREFIX)-darwin-arm64: OPTS := GOOS=darwin GOARCH=arm64
$(TARGET_PREFIX)-darwin-arm64: BUILD_SUFFIX := darwin-arm64

$(TARGET_PREFIX)-linux-x86_64: OPTS := GOOS=linux GOARCH=amd64
$(TARGET_PREFIX)-linux-x86_64: BUILD_SUFFIX := linux-x86_64

$(TARGET_PREFIX)-linux-arm64: OPTS := GOOS=linux GOARCH=arm64
$(TARGET_PREFIX)-linux-arm64: BUILD_SUFFIX := linux-arm64

$(TARGET_PREFIX)-noarch: BUILD_SUFFIX := noarch
