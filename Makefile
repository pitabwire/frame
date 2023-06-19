SHELL := /bin/bash

.PHONY: help clean build

.DEFAULT_GOAL := help

ENV_LOCAL_TEST=\
  TEST_DATABASE_URL=postgres://frame:secret@localhost:5431/framedatabase?sslmode=disable \
  POSTGRES_PASSWORD=secret \
  POSTGRES_DB=service_notification \
  POSTGRES_HOST=notification_db \
  POSTGRES_USER=ant

SERVICE		?= $(shell basename `go list`)
VERSION		?= $(shell git describe --tags --always --dirty --match=v* 2> /dev/null || cat $(PWD)/.version 2> /dev/null || echo v0)
PACKAGE		?= $(shell go list)
PACKAGES	?= $(shell go list ./...)
FILES		?= $(shell find . -type f -name '*.go' -not -path "./vendor/*")

help:   ## show this help
	@echo 'usage: make [target] ...'
	@echo ''
	@echo 'targets:'
	@egrep '^(.+)\:\ .*##\ (.+)' ${MAKEFILE_LIST} | sed 's/:.*##/#/' | column -t -c 2 -s '#'

clean:  ## go clean
	go clean

fmt:    ## format the go source files
	go fmt ./...

vet:    ## run go vet on the source files
	go vet ./...

doc:    ## generate godocs and start a local documentation webserver on port 8085
	godoc -http=:8085 -index

# this command will start docker components that we set in docker-compose.yml
docker-setup: ## sets up docker container images
	docker-compose -f tests_runner/docker-compose.yml up -d

# shutting down docker components
docker-stop: ## stops all docker containers
	docker-compose -f tests_runner/docker-compose.yml down

# this command will run all tests in the repo
# INTEGRATION_TEST_SUITE_PATH is used to run specific tests in Golang,
# if it's not specified it will run all tests
tests: ## runs all system tests
	$(ENV_LOCAL_TEST) \
	FILES=$(go list ./...  | grep -v /vendor/);\
	go test ./... -v -run=$(INTEGRATION_TEST_SUITE_PATH)  -coverprofile=coverage.out;\
	RETURNCODE=$$?;\
	if [ "$$RETURNCODE" -ne 0 ]; then\
		echo "unit tests failed with error code: $$RETURNCODE" >&2;\
		exit 1;\
	fi;\
	go tool cover -html=coverage.out -o coverage.html


gen_tls_certificates:
	rm -f tests_runner/*.pem
	# Generate ca file

	openssl req -x509 -newkey rsa:4096 -days 365 -nodes \
			-keyout tests_runner/ca-key.pem -out tests_runner/ca-cert.pem \
			-subj "/C=CN/ST=Busia/L=Kisenyi B/O=Testing Systems Ltd, Inc./OU=Systems /CN=localhost"


	# Generate csr
	openssl req -newkey rsa:4096 -nodes -keyout tests_runner/server-key.pem -out tests_runner/server-req.pem \
			-subj "/C=CN/ST=Busia/L=Kisenyi B/O=Testing Systems Ltd, Inc./OU=Systems /CN=localhost"


	# Generate server certificate
	openssl x509 -req -extfile <(printf "subjectAltName=DNS:localhost")  -in tests_runner/server-req.pem -days 60 -CA tests_runner/ca-cert.pem \
			-CAkey tests_runner/ca-key.pem -CAcreateserial -out tests_runner/server-cert.pem


build: clean fmt vet gen_tls_certificates docker-setup tests docker-stop ## run all preliminary steps and tests the setup
