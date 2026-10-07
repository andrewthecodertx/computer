# computer — Go backend. Requires the `linkos-pg` PostgreSQL container.
#
# make run          ensure DB is up, then build and run the Go app (dev)
# make docker       build + start the app container (db must already be up)
# make docker-down  stop the app container
# make db-up        start (or wait on) the PostgreSQL container
# make db-down      stop the container (data persists in the named volume)
# make db-shell     psql into the computer database
# make db-init      (fresh) drop/create + apply db/schema.sql
# make build       go build ./...
# make vet         go vet ./...
# make migrate     apply schema/migrations (TODO: implement in internal/db)

DB_CONTAINER ?= linkos-pg
DB_URL       ?= postgres://linkos:linkos@localhost:5432/computer?sslmode=disable
COMPOSE      ?= docker compose

.PHONY: run docker docker-down db-up db-down db-shell db-init build vet migrate

run: db-up
	@echo ">> building and running go-computer (DATABASE_URL=$(DB_URL))"
	@DATABASE_URL="$(DB_URL)" go build -o /tmp/go-computer . && DATABASE_URL="$(DB_URL)" /tmp/go-computer

docker: db-up
	@echo ">> building and starting the app container"
	@$(COMPOSE) build app
	@$(COMPOSE) up -d --force-recreate app

docker-down:
	@$(COMPOSE) down

db-up:
	@echo ">> ensuring $(DB_CONTAINER) is running"
	@if [ "$$(docker inspect -f '{{.State.Running}}' $(DB_CONTAINER) 2>/dev/null)" != "true" ]; then \
		docker compose -f docker-compose.full.yml up -d db; \
	fi
	@echo ">> waiting for $(DB_CONTAINER) to accept connections"
	@for i in $$(seq 1 30); do \
		if docker exec $(DB_CONTAINER) pg_isready -U computer -d computer >/dev/null 2>&1; then \
			echo ">> database ready"; exit 0; \
		fi; \
		sleep 1; \
	done; \
	echo ">> database did not become ready"; exit 1

db-down:
	docker stop $(DB_CONTAINER) || true

db-shell:
	docker exec -it $(DB_CONTAINER) psql -U computer -d computer

db-init:
	@echo ">> dropping and recreating the computer database"
	@cat db/schema.sql | docker exec -i $(DB_CONTAINER) psql -U linkos -d postgres
	@echo ">> schema applied"

build:
	go build ./...

vet:
	go vet ./...

migrate:
	@echo "TODO: apply schema/migrations in internal/db"
	@echo "Manual: cat db/schema.sql | docker exec -i $(DB_CONTAINER) psql -U linkos -d postgres"