VENV := .venv
PYTHON := $(VENV)/bin/python3
PIP := $(VENV)/bin/pip
PYTEST := $(VENV)/bin/pytest

QUALIFICATION_REPETITIONS ?= 3
QUALIFICATION_OUTPUT_DIR ?= testresults/agent-qualification

.PHONY: install test test-benchmark test-qualification qualify-agent

install:
	python3 -m venv $(VENV)
	$(PIP) install -e ".[dev]" -q

test:
	$(PYTEST) tests/ -v

test-benchmark:
	$(PIP) install -e ".[dev]" -q
	$(PYTEST) benchmark/tests/ -v

test-qualification:
	$(PYTEST) qualification/tests/ -v

qualify-agent:
	@mkdir -p $(QUALIFICATION_OUTPUT_DIR)
	$(PYTHON) -m qualification \
		--repetitions $(QUALIFICATION_REPETITIONS) \
		--output-dir $(QUALIFICATION_OUTPUT_DIR)
