.PHONY: help build-backend test-backend build-frontend build-all run-backend run-frontend clean docker-up docker-down docker-restart docker-rebuild docker-logs

help:
	@echo "Domain Monitoring Dashboard - Makefile"
	@echo "----------------------------------------------------"
	@echo "make build-backend   - Build the Go agent binary"
	@echo "make test-backend    - Run the Go unit tests"
	@echo "make build-frontend  - Build the static Vue 3 dist/"
	@echo "make build-all       - Test, then build both"
	@echo "make run-backend     - Run the agent locally on :9292"
	@echo "make run-frontend    - Run the Vite dev server on :3001"
	@echo "make clean           - Remove build artifacts"
	@echo ""
	@echo "Docker (central server):"
	@echo "make docker-up       - Build images and start agent + dashboard"
	@echo "make docker-down     - Stop and remove both containers"
	@echo "make docker-restart  - Restart to pick up .env changes (no rebuild)"
	@echo "make docker-rebuild  - Rebuild from scratch after changing source"
	@echo "make docker-logs     - Follow container logs"

build-backend:
	@echo "==> Building the Go agent..."
	cd backend && go build -ldflags="-s -w" -o bin/domain-monitor ./cmd/agent

test-backend:
	@echo "==> Running Go unit tests..."
	cd backend && go test ./... -v

build-frontend:
	@echo "==> Building the Vue 3 static distribution..."
	cd frontend && npm run build

build-all: test-backend build-backend build-frontend
	@echo "==> All builds completed successfully!"

# The agent reads list-domain.txt from the repository directly in development,
# and writes its history under backend/data/ rather than the container volume.
run-backend:
	@echo "==> Starting the domain agent on :9292..."
	cd backend && go run ./cmd/agent --targets ../list-domain.txt --db ./data/domain-monitor.db

run-frontend:
	@echo "==> Starting the Vite development server on :3001..."
	cd frontend && npm run dev

clean:
	@echo "==> Cleaning build artifacts..."
	rm -rf backend/bin backend/data frontend/dist

# --- Docker (central server) -----------------------------------------------

docker-up:
	@echo "==> Starting agent and dashboard..."
	docker compose up -d --build
	@echo "==> Up. Check with: make docker-logs"

docker-down:
	@echo "==> Stopping containers..."
	docker compose down

# Thresholds, intervals and the published port are read at container start, so
# an .env change only needs a restart, never an image rebuild.
docker-restart:
	@echo "==> Restarting to apply .env changes..."
	docker compose up -d

docker-rebuild:
	@echo "==> Rebuilding images without cache..."
	docker compose build --no-cache
	docker compose up -d

docker-logs:
	docker compose logs -f
