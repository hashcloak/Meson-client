
all: build

build:
	docker build -t hashcloak/meson-client -f Dockerfile .
