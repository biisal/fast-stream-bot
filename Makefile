# Variables
GREETING := Hello, world!

# Phony targets to ensure make doesn't confuse them with files
.PHONY: default backend-build frontend-build run backend-dev frontend-dev dev help

default:
	@echo "$(GREETING)"

backend-build:
	go build -o fast-stream-bot ./cmd/fsb

frontend-build:
	pnpx @tailwindcss/cli -i ./frontend/assets/styles/input.css -o ./frontend/assets/styles/tailwind.css

run: backend-build frontend-build
	go run ./cmd/fsb

backend-dev:
	air -c air.toml

frontend-dev:
	pnpx @tailwindcss/cli -i ./frontend/assets/styles/input.css -o ./frontend/assets/styles/tailwind.css --watch

dev:
	@echo "Running backend and frontend in development mode..."
	# Using make -j2 to run targets in parallel
	$(MAKE) -j2 backend-dev frontend-dev
