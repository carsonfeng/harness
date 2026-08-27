GO ?= go
REMOTE ?= origin

.PHONY: test tag

test:
	@$(GO) test ./...

tag:
	@if [ -z "$(m)" ]; then \
		printf 'Usage: make tag m="release message"\n'; \
		exit 1; \
	fi
	@if [ -n "$$(git status --porcelain)" ]; then \
		printf 'Commit your changes before creating a tag.\n'; \
		exit 1; \
	fi
	@$(GO) test ./...
	@git fetch --quiet "$(REMOTE)" --tags
	@latest="$$(git tag --list 'v*' --sort=-version:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$$' | head -n 1)"; \
	if [ -z "$$latest" ]; then \
		next="v0.1.0"; \
	else \
		next="$$(printf '%s\n' "$$latest" | awk -F. '{printf "v%d.%d.%d", substr($$1, 2), $$2, $$3 + 1}')"; \
	fi; \
	git tag -a "$$next" -m "$(m)"; \
	git push "$(REMOTE)" "$$next"; \
	printf 'Released %s: %s\n' "$$next" "$(m)"
