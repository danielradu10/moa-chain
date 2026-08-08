VENV := .venv
PYTHON := $(VENV)/bin/python3
PIP := $(VENV)/bin/pip
PYTEST := $(VENV)/bin/pytest

.PHONY: install test test-benchmark

install:
	python3 -m venv $(VENV)
	$(PIP) install -e ".[dev]" -q

test:
	$(PYTEST) tests/ -v

test-benchmark:
	$(PIP) install -e ".[dev]" -q
	$(PYTEST) benchmark/tests/ -v
