include .env
export $(shell sed 's/=.*//' .env)

MGDIR = sql/mangapark/migration
up:
	goose -dir $(MGDIR) postgres ${DB_URL} up

create_%:
	goose -dir $(MGDIR) create $* sql

status:
	goose -dir $(MGDIR) postgres ${DB_URL} status

reset:
	goose -dir $(MGDIR) postgres ${DB_URL} reset
	goose -dir $(MGDIR) postgres ${DB_URL} up

down:
	goose -dir $(MGDIR) postgres ${DB_URL} down

swag:
	swag init -g cmd/main.go