.PHONY: run docker stop docker-down db-up db-shell build vet test integration db-converge check audit e2e logs migrate seed deploy
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
# schema.sql <-> migration-chain convergence + runner atomicity proofs
# (review MIG-03): fails when the two paths to the same schema drift apart
# or when apply/record atomicity regresses. Scratch databases only.
db-converge:
	docker compose run --rm converge
	docker compose run --rm converge sh /db/migrate-atomicity.sh
# Web toolchain command; override when yarn comes through corepack:
#   make check YARN="corepack yarn"
YARN ?= yarn
# Release validation (review TEST-02): the database-backed suite is
# mandatory here — unlike `make test`, this target fails when Docker (and
# therefore the PostgreSQL integration coverage) is unavailable instead of
# quietly accepting skipped tests.
check:
	@docker compose ps >/dev/null 2>&1 || { echo "error: release checks require docker; database-backed coverage is mandatory" >&2; exit 1; }
	$(MAKE) build
	$(MAKE) vet
	$(MAKE) integration
	$(MAKE) db-converge
	cd web && $(YARN) lint && $(YARN) eslint -c eslint.ssr.config.mjs . && $(YARN) build
	@[ -d scripts/node_modules ] || npm --prefix scripts ci
	npm --prefix scripts run test:regressions
	@echo "release check complete: vet + database integration + schema convergence + web lint/build + browser regressions"
	@echo "note: 'make e2e' (real-stack browser smoke tests) is a separate step; run it against the composed stack before release."
# Dependency advisory scanning (review "Dependency assessment boundary"):
# opt-in because it needs network access and the Go vuln tool installed
# (go install golang.org/x/vuln/cmd/govulncheck@latest).
audit:
	@command -v govulncheck >/dev/null 2>&1 || { echo "error: govulncheck not installed: go install golang.org/x/vuln/cmd/govulncheck@latest" >&2; exit 1; }
	govulncheck ./internal/... .
	cd web && $(YARN) audit
	npm --prefix scripts audit
e2e:
	@[ -d scripts/node_modules ] || npm --prefix scripts ci
	npm --prefix scripts test
logs:
	docker compose logs -f app web
migrate:
	docker compose run --rm migrate
seed:
	docker compose exec -e SEED_EMAIL -e SEED_PASSWORD -e SEED_NAME app /computer --seed
# Target-agnostic server deploy; requires HOST, DOMAIN and CERTBOT_EMAIL
# (no defaults). See README "Deploying to a server".
deploy:
	./scripts/deploy.sh
