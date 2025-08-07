# Makefile for s6-benthos integration test

.PHONY: build run clean test logs shell help

# Default target
help:
	@echo "Available targets:"
	@echo "  build   - Build the Docker image"
	@echo "  run     - Run the test in a container"
	@echo "  test    - Build and run the test"
	@echo "  logs    - Show logs from the running container"
	@echo "  shell   - Open a shell in the container"
	@echo "  clean   - Remove built images and containers"
	@echo "  help    - Show this help message"

# Build the Docker image
build:
	@echo "Building Docker image..."
	docker build -t s6-benthos-test .

# Run the test in a container
run:
	@echo "Running s6-benthos test..."
	docker run --rm --name s6-benthos-test \
		-p 8080-8090:8080-8090 \
		-p 9080-9090:9080-9090 \
		--ulimit nofile=30000:30000 \
		s6-benthos-test

# Build and run the test
test: build run

# Show logs from a running container
logs:
	docker logs -f s6-benthos-test

# Open a shell in the container for debugging
shell:
	docker run --rm -it --name s6-benthos-test-shell \
		-p 8080-8090:8080-8090 \
		-p 9080-9090:9080-9090 \
		--entrypoint /bin/bash \
		s6-benthos-test

# Clean up built images and containers
clean:
	@echo "Cleaning up..."
	-docker stop s6-benthos-test 2>/dev/null || true
	-docker rm s6-benthos-test 2>/dev/null || true
	-docker rmi s6-benthos-test 2>/dev/null || true

# Run with verbose s6 logging
run-verbose:
	@echo "Running s6-benthos test with verbose logging..."
	docker run --rm --name s6-benthos-test \
		-e S6_VERBOSITY=3 \
		-p 8080-8090:8080-8090 \
		-p 9080-9090:9080-9090 \
		s6-benthos-test

# Run in background
run-background:
	@echo "Running s6-benthos test in background..."
	docker run -d --name s6-benthos-test \
		-p 8080-8090:8080-8090 \
		-p 9080-9090:9080-9090 \
		s6-benthos-test

# Stop background container
stop:
	docker stop s6-benthos-test || true
	docker rm s6-benthos-test || true
