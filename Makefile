# Copyright (c) 2018 TFG Co <backend@tfgco.com>
# Author: TFG Co <backend@tfgco.com>
#
# Permission is hereby granted, free of charge, to any person obtaining a copy of
# this software and associated documentation files (the "Software"), to deal in
# the Software without restriction, including without limitation the rights to
# use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
# the Software, and to permit persons to whom the Software is furnished to do so,
# subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
# FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
# COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
# IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
# CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

build:
	@mkdir -p bin
	@go build -o bin/pusher main.go

dep:
	@go get -u github.com/golang/dep/cmd/dep

setup-ci-deps:
	@go get github.com/mattn/goveralls
	@go get github.com/onsi/ginkgo/ginkgo
	@dep ensure

setup-ci: dep setup-ci-deps

start-deps:
	@echo "Starting dependencies..."
	@docker compose --project-name pusher up --wait --renew-anon-volumes --build
	@echo "Dependencies started successfully."

stop-deps:
	@docker compose --project-name pusher down --remove-orphans --volumes

rtfd:
	@rm -rf docs/_build
	@sphinx-build -b html -d ./docs/_build/doctrees ./docs/ docs/_build/html
	@open docs/_build/html/index.html

run:
	@go run main.go

gcm:
	@go run main.go gcm --senderId=test --apiKey=123

apns:
	@go run main.go apns --certificate=./tls/_fixtures/certificate-valid.pem

local-deps:
	@docker compose --project-name pusher up --wait --renew-anon-volumes --build

setup:
	# Ensuring librdkafka is installed in Mac OS
	@/bin/bash -c '[ "`uname -s`" == "Darwin" ] && [ "`which brew`" != "" ] && [ ! -d "/usr/local/Cellar/librdkafka" ] && echo "librdkafka was not found. Installing with brew..." && brew install librdkafka; exit 0'
	# Ensuring librdkafka is installed in Debian and Ubuntu
	@/bin/bash -c '[ "`uname -s`" == "Linux" ] && [ "`which apt-get`" != "" ] && echo "Ensuring librdkafka is installed..." && ./debian-install-librdkafka.sh; exit 0'
	@go get -u github.com/golang/dep/cmd/dep
	@go get -u github.com/onsi/ginkgo/ginkgo
	@go get github.com/gordonklaus/ineffassign
	@dep ensure

test: test-unit test-integration

test-coverage-func:
	@mkdir -p _build
	@-rm -rf _build/test-coverage-all.out
	@echo "mode: count" > _build/test-coverage-all.out
	@bash -c 'for f in $$(find . -name "*.coverprofile"); do tail -n +2 $$f >> _build/test-coverage-all.out; done'
	@echo
	@echo "=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-"
	@echo "Functions NOT COVERED by Tests"
	@echo "=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-"
	@go tool cover -func=_build/test-coverage-all.out | egrep -v "100.0[%]"

test-coverage: test test-coverage-run

test-coverage-run:
	@mkdir -p _build
	@-rm -rf _build/test-coverage-all.out
	@echo "mode: count" > _build/test-coverage-all.out
	@bash -c 'for f in $$(find . -name "*.coverprofile"); do tail -n +2 $$f >> _build/test-coverage-all.out; done'

test-coverage-html cover:
	@go tool cover -html=_build/test-coverage-all.out

test-coverage-write-html:
	@go tool cover -html=_build/test-coverage-all.out -o _build/test-coverage.html

test-services: stop-deps start-deps test-db-drop test-db-create
	@echo "Required test services are up."

test-db-drop:
	@psql -U postgres -h localhost -p 8585 -f db/drop-test.sql > /dev/null

test-db-create:
	@psql -U postgres -h localhost -p 8585 -f db/create-test.sql > /dev/null

test-unit unit:
	@echo
	@echo "-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-="
	@echo "=                  Running unit tests...                 ="
	@echo "-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-="
	@echo
	@ginkgo -v -r --randomizeAllSpecs --randomizeSuites --cover --focus="\[Unit\].*" .
	@$(MAKE) test-coverage-func
	@echo
	@echo "-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-="
	@echo "=                  Unit tests finished.                  ="
	@echo "-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-="
	@echo

run-integration-test:
	@echo
	@echo "-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-="
	@echo "=               Running integration tests...             ="
	@echo "-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-="
	@echo
	@ginkgo -v -r --randomizeSuites --focus="\[Integration\].*" .
	@echo
	@echo "-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-="
	@echo "=               Integration tests finished.              ="
	@echo "-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-="
	@echo

test-integration: start-deps test-db-drop test-db-create run-integration-test

lint:
	@golangci-lint run

build-image-dev:
	@docker build -f Dockerfile.local -t pusher:local .

lint-container-dev: build-image-dev
	@docker run \
		--rm \
		--volume "${PWD}":/go/src/github.com/topfreegames/pusher \
		pusher:local golangci-lint run

build-container-dev: build-image-dev
	@docker run \
		--rm \
		--volume "${PWD}":/go/src/github.com/topfreegames/pusher \
		pusher:local bash -c 'dep ensure && make build'

unit-test-container-dev: build-image-dev
	@docker run \
		--rm \
		--volume "${PWD}":/go/src/github.com/topfreegames/pusher \
		pusher:local bash -c 'dep ensure && make unit'

start-deps-container-dev:
	@echo "Starting dependencies..."
	@docker compose -f docker-compose-container-dev.yml --project-name pusher up --wait --renew-anon-volumes --build
	@echo "Dependencies started successfully."

integration-test-container-dev: build-image-dev start-deps-container-dev test-db-drop test-db-create
	@docker run \
		-t \
		-i \
		--rm \
		--network pusher_default \
		-e CONFIG_FILE="../config/docker_test.yaml" \
		--volume "${PWD}":/go/src/github.com/topfreegames/pusher \
		pusher:local make run-integration-test
	@$(MAKE) stop-deps
