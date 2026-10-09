.PHONY: run docker stop docker-down db-up db-shell build vet test integration e2e logs migrate seed
run docker:
	docker compose up -d --build
stop docker-down:
	docker compose down
db-up:
	docker compose up -d db
db-shell:
	docker compose exec db psql -U computer -d computer
build:
	go build ./internal/... .
vet:
	go vet ./internal/... .
test:
	@if [ -z "$$TEST_DATABASE_URL" ]; then \
		echo "warning: TEST_DATABASE_URL is unset - PostgreSQL integration tests will SKIP."; \
		echo "         run 'make integration' for full coverage."; \
	fi
	go test -race ./internal/...
integration:
	docker compose run --rm tests
e2e:
	@[ -d scripts/node_modules ] || npm --prefix scripts ci
	npm --prefix scripts test
logs:
	docker compose logs -f app web
migrate:
	docker compose run --rm migrate
seed:
	docker compose exec -e SEED_EMAIL -e SEED_PASSWORD -e SEED_NAME app /computer --seed
