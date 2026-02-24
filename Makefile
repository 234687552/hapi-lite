.PHONY: dev build web install-web build-web build-all

dev:
	go run main.go

web:
	cd web && npm run dev

build:
	go build -o hapi-lite main.go

build-web:
	cd web && npm run build

build-all: build-web build

install-web:
	cd web && npm install
