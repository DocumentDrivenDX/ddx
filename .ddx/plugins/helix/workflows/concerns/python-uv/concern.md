# Concern: Python + uv

## Category
tech-stack

## Areas
all

## Components

- **Language**: Python 3.12+
- **Package manager**: uv — NOT pip, NOT poetry, NOT conda
- **Virtual environment**: uv-managed (`.venv` via `uv sync`)
- **Build backend**: `hatchling` with `uv-dynamic-versioning` for versioned packages
- **Linter**: `ruff` — NOT flake8, NOT pylint
- **Type checker**: `pyright` — NOT mypy
- **Test framework**: `pytest` with `pytest-cov`
- **Property-based testing**: `hypothesis`

## Constraints

- All code must pass `pyright` type checking
- All code must pass `ruff check` and `ruff format --check`
- Use `pyproject.toml` for all project metadata (not `setup.py`, not `setup.cfg`)
- Pin Python version in `.python-version`
- All dependencies in `[project.dependencies]` or `[dependency-groups]` (dev); no `requirements.txt`
- Use `[tool.uv.sources]` for custom package indexes (e.g., PyTorch CUDA wheels)
- Tests in `tests/` directory; pytest markers for test categories (acceptance, contract, slow, fast)
- Branch coverage enforced via `pytest-cov` with `fail_under`

## When to use

Python projects that benefit from fast dependency resolution and reproducible
environments. Good for data services, APIs, ML pipelines, CLI tools, and
libraries. uv is the single tool for venv creation, dependency resolution,
and script running.

## ADR References
