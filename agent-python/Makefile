VENV := .venv
PYTHON := $(VENV)/bin/python3
PIP := $(VENV)/bin/pip
PYTEST := $(VENV)/bin/pytest

.PHONY: install test

install:
	python3 -m venv $(VENV)
	$(PIP) install -e ".[dev]" -q

test:
	$(PYTEST) tests/ -v
