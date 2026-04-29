# Practices: python-uv

## Requirements (Frame phase)
- Specify minimum Python version (3.12+ preferred)
- Identify whether the project is a library (published) or an application (not published)
- If ML/GPU dependencies exist, plan for `[tool.uv.sources]` with custom package indexes

## Design
- One `pyproject.toml` per project (or per package in a workspace)
- Library projects: use `hatchling` build backend with `uv-dynamic-versioning` for git-tag-based versions
- Application projects: `[tool.uv] package = false` (no build artifact needed)
- Organize source under `src/<package_name>/` layout
- Use `pydantic` v2 for data validation and settings models
- Use `typer` + `rich` for CLI interfaces

## Implementation
- Create/sync environment: `uv sync` (creates `.venv` automatically)
- Run scripts: `uv run python ...` or `uv run pytest`
- Add dependencies: `uv add <pkg>` (not `pip install`)
- Add dev dependencies: `uv add --dev <pkg>` or to `[dependency-groups] dev`
- Type annotations: all public functions and methods must have type annotations
- Avoid `Any` — use `pyright` targeted `# type: ignore` with comment when unavoidable
- Use `TYPE_CHECKING` guard for import-only type imports

## Testing
- Framework: `pytest`
- Run: `uv run pytest`
- Property-based: `hypothesis` for data invariants and input space exploration
- Mocking: `pytest-mock` (not `unittest.mock` directly)
- Coverage: `pytest-cov` with branch coverage; set `fail_under` in `[tool.coverage.report]`
- Test markers: `acceptance`, `contract`, `slow`, `fast` — use `--strict-markers`
- Filter known third-party deprecation warnings in `[tool.pytest.ini_options] filterwarnings`

## Quality Gates (pre-commit / CI)
- `ruff check .` — lint
- `ruff format --check .` — format
- `pyright` — type check
- `uv run pytest --cov` — tests with coverage
- `pre-commit run --all-files` for the full gate

## Dependency Management
- `uv add <pkg>` / `uv add --dev <pkg>`
- Custom indexes: declare in `[tool.uv.sources]` and `[[tool.uv.index]]`
- Lock file: `uv.lock` committed
- Do not commit `.venv/` or use system Python
