# Copyright (c) 2016 TFG Co <backend@tfgco.com>
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

test-services: stop-deps deps test-db-drop test-db-create
	@echo "Required test services are up."

test-db-drop:
	@psql -U postgres -h localhost -p 8585 -f db/drop-test.sql > /dev/null

test-db-create:
	@psql -U postgres -h localhost -p 8585 -f db/create-test.sql > /dev/null

test-unit unit: stop-deps
	@echo
	@echo "-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-="
	@echo "=                  Running unit tests...                 ="
	@echo "-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-="
	@echo
	@env MY_IP=${MY_IP} ginkgo -r --randomizeAllSpecs --randomizeSuites --cover --focus="\[Unit\].*" .
	@$(MAKE) test-coverage-func
	@echo
	@echo "-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-="
	@echo "=                  Unit tests finished.                  ="
	@echo "-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-="
	@echo

test-integration integration func: deps test-db-drop test-db-create
	@echo
	@echo "-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-="
	@echo "=               Running integration tests...             ="
	@echo "-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-="
	@echo
	@env MY_IP=${MY_IP} ginkgo -r --randomizeAllSpecs --randomizeSuites --skip="\[Integration\].*" .
	@echo
	@echo "-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-="
	@echo "=               Integration tests finished.              ="
	@echo "-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-="
	@echo
