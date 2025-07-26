# PostgreSQL Docker Makefile

# Configuration
POSTGRES_USER ?= postgres
POSTGRES_PASSWORD ?= postgres
POSTGRES_PORT ?= 15432
POSTGRES_DB ?= postgres
CONTAINER_NAME ?= postgres-container
DATA_VOLUME ?= postgres-data

start:
	@echo "Starting PostgreSQL container..."
	@if [ -z "$$(docker ps -aq -f name=$(CONTAINER_NAME))" ]; then \
		docker run -d \
			--name $(CONTAINER_NAME) \
			-e POSTGRES_USER=$(POSTGRES_USER) \
			-e POSTGRES_PASSWORD=$(POSTGRES_PASSWORD) \
			-e POSTGRES_DB=$(POSTGRES_DB) \
			-p $(POSTGRES_PORT):5432 \
			-v $(DATA_VOLUME):/var/lib/postgresql/data \
			postgres:latest; \
	elif [ -z "$$(docker ps -q -f name=$(CONTAINER_NAME))" ]; then \
		docker start $(CONTAINER_NAME); \
	fi
	@echo "PostgreSQL is running on port $(POSTGRES_PORT)"

stop:
	@echo "Stopping PostgreSQL container..."
	@docker stop $(CONTAINER_NAME) 2>/dev/null || echo "Container $(CONTAINER_NAME) not running"
	@echo "PostgreSQL stopped"

reset-data: stop
	@echo "Removing old PostgreSQL container and data volume..."
	@docker rm $(CONTAINER_NAME) 2>/dev/null || echo "Container $(CONTAINER_NAME) does not exist"
	@docker volume rm $(DATA_VOLUME) 2>/dev/null || echo "Volume $(DATA_VOLUME) does not exist"
	@echo "Data has been reset. Run 'make start' to start with fresh database."

psql:
	@echo "Connecting to PostgreSQL with psql..."
	@docker exec -it $(CONTAINER_NAME) psql -U $(POSTGRES_USER) -d $(POSTGRES_DB)

status:
	@docker ps -f name=$(CONTAINER_NAME)

logs:
	@docker logs -f $(CONTAINER_NAME)

reset-then-start: reset-data start
	sleep 1
	go run service.go

.PHONY: start stop reset-data status logs