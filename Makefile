SHELL := /bin/bash
.NOTPARALLEL:

DAGGER ?= dagger
LOCAL_MODULE ?= .

.PHONY: ci lint test test-race vulncheck web web-test build dev verify-web-dist \
        dagger-develop dev-native e2e clean

## Canonical CI gate (identical locally and in GitHub Actions)
ci:
	$(DAGGER) call -m $(LOCAL_MODULE) ci export --path .
	@test -f ./webui/dist/index.html || (echo "ERROR: ./webui/dist/index.html missing after export" && exit 1)
	$(MAKE) verify-web-dist

## Regenerate local Dagger SDK bindings after editing dagger.json or dagger/*.go
dagger-develop:
	$(DAGGER) develop

lint:
	$(DAGGER) call -m $(LOCAL_MODULE) lint

test:
	$(DAGGER) call -m $(LOCAL_MODULE) test

test-race:
	$(DAGGER) call -m $(LOCAL_MODULE) test-race

vulncheck:
	$(DAGGER) call -m $(LOCAL_MODULE) vulncheck

web:
	$(DAGGER) call -m $(LOCAL_MODULE) web-build export --path ./webui/dist
	@test -f ./webui/dist/index.html || (echo "ERROR: ./webui/dist/index.html missing after export" && exit 1)

web-test:
	$(DAGGER) call -m $(LOCAL_MODULE) web-test

build:
	$(DAGGER) call -m $(LOCAL_MODULE) build export --path ./bin

## Run simulator + embedded UI as a Dagger service on http://127.0.0.1:8080 (Ctrl-C to stop)
dev:
	$(DAGGER) call -m $(LOCAL_MODULE) dev up --ports 8080:8080

## Verify committed webui/dist matches freshly generated Nuxt bundle (including untracked hashed files)
verify-web-dist:
	@test -z "$$(git status --porcelain -- webui/dist)" || (echo "webui/dist is out of date; run 'make web' and commit the result" && git status --porcelain -- webui/dist && exit 1)

## Optional native convenience targets for local human debugging / Playwright E2E
dev-native:
	go run ./cmd/gobot-simulate --port 8080

e2e: build
	cd web && npx playwright test

clean:
	rm -rf bin/ coverage.out web/.nuxt web/.output
