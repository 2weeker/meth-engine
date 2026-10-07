COMPOSE ?= docker compose
BIN     := bin/meth

.DEFAULT_GOAL := up
.PHONY: deploy update check health up dev down restart rebuild logs ps shell db reset backup restore \
        generate build run test vet fmt clean

deploy:
	@if [ -d config.yaml ]; then \
		echo "config.yaml is a directory. Remove it (rmdir config.yaml) and run 'make deploy' again."; exit 1; fi
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		echo "Created .env from .env.example. Fill in its secrets and METH_DOMAIN, then run 'make deploy' again."; \
		exit 1; fi
	@if [ ! -f config.yaml ]; then \
		cp config.example.yaml config.yaml; \
		echo "Created config.yaml from config.example.yaml. Edit it, then run 'make deploy' again."; \
		exit 1; fi
	$(COMPOSE) config --quiet
	$(COMPOSE) build
	$(COMPOSE) run --rm --no-deps -T meth -check
	@$(COMPOSE) up -d --wait --remove-orphans || { \
		echo; echo "The deploy did not come up healthy. The engine's last log lines:"; \
		$(COMPOSE) logs --tail=30 meth; exit 1; }
	@$(COMPOSE) ps
	@echo "meth is deployed and healthy.   (make logs to follow)"

update:
	git pull --ff-only
	$(MAKE) deploy

check:
	$(COMPOSE) run --rm --no-deps -T meth -check

health:
	@$(COMPOSE) exec -T meth /meth -healthcheck
	@$(COMPOSE) ps

up:
	$(COMPOSE) up --build -d
	@echo "meth is up: http://localhost:3000   (make logs to follow)"

dev:
	$(COMPOSE) up --build

down:
	$(COMPOSE) down

restart:
	$(COMPOSE) restart meth

rebuild:
	$(COMPOSE) build --no-cache meth
	$(COMPOSE) up -d meth

logs:
	$(COMPOSE) logs -f meth

ps:
	$(COMPOSE) ps

shell:
	$(COMPOSE) exec db psql -U meth -d meth

db:
	$(COMPOSE) up -d db
	@echo "postgres://meth:meth@localhost:5434/meth"

reset:
	@printf "This deletes every post, ban and user. Type yes to continue: "; \
	read ans; [ "$$ans" = "yes" ] || { echo "aborted"; exit 1; }
	$(COMPOSE) down -v

backup:
	@mkdir -p backups
	@$(COMPOSE) up -d --wait db
	@stamp=$$(date +%Y%m%d-%H%M%S); out=backups/meth-$$stamp.dump; \
	$(COMPOSE) exec -T db pg_dump -U meth -d meth --format=custom > $$out || { rm -f $$out; echo "backup failed"; exit 1; }; \
	if [ -f config.yaml ]; then cp config.yaml backups/meth-$$stamp.config.yaml; fi; \
	echo "Backed up to $$out ($$(du -h $$out | cut -f1))"

restore:
	@file="$(FILE)"; \
	if [ -z "$$file" ]; then file=$$(ls -1t backups/*.dump 2>/dev/null | head -n 1); fi; \
	if [ ! -f "$$file" ]; then echo "No backup found. Use: make restore FILE=backups/meth-YYYYMMDD-HHMMSS.dump"; exit 1; fi; \
	printf "This replaces every post, ban, filter and user with %s. Type yes to continue: " "$$file"; \
	read ans; [ "$$ans" = "yes" ] || { echo "aborted"; exit 1; }; \
	$(COMPOSE) up -d --wait db || exit 1; \
	$(COMPOSE) stop meth; \
	$(COMPOSE) exec -T db pg_restore -U meth -d meth --clean --if-exists --no-owner --single-transaction < "$$file"; \
	status=$$?; \
	$(COMPOSE) up -d meth; \
	if [ $$status -ne 0 ]; then echo "restore failed; the database was left as it was"; exit $$status; fi; \
	echo "Restored $$file"

generate:
	go tool templ generate

build: generate
	go build -o $(BIN) .

run: build
	@test -f .env || { echo "No .env: copy .env.example to .env and fill it in."; exit 1; }
	set -a; . ./.env; set +a; \
	DATABASE_URL=postgres://meth:$${POSTGRES_PASSWORD}@localhost:$${METH_DB_PORT:-5434}/meth \
	./$(BIN)

test: generate
	go test ./...

vet: generate
	@test -z "$$(gofmt -l . | tee /dev/stderr)" || { echo "gofmt the files above"; exit 1; }
	go vet ./...

fmt:
	gofmt -w .
	go tool templ fmt .

clean:
	rm -rf bin
