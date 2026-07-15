.PHONY: setup
setup: ## Install the pinned toolchain (mise.toml) and register the pre-commit git hooks.
	mise install
	mise exec -- pre-commit install
