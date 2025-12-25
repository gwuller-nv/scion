.PHONY: build clean

build:
	@echo "Building scion..."
	@go build -ldflags "$$(./hack/version.sh)" -o scion main.go

clean:
	@rm -f scion
