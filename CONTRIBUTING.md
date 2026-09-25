**English** | [Русский](CONTRIBUTING.ru.md)

# Contributing

Thanks for wanting to help! Here's how work on the project is organized.

## Reporting a problem

Open an [issue](https://github.com/DragonZero000/Manga-Reader/issues) with: the app version (**Настройки** (Settings) → **О приложении** (About)), the platform (Windows / Android and its version), what you did, what you expected and what happened. If the problem is with a particular archive, describe its contents (file list, whether it has a `meta.json`); there's no need to attach the archive itself.

## Proposing a change

1. Fork the repository and create a branch from `main`.
2. Build and run the project — see [docs/en/building.md](docs/en/building.md).
3. For a noticeable change (a new feature, a behavior change), describe it first in an issue or via OpenSpec (below). Small fixes can be sent right away.
4. Run `make test` before opening a pull request — CI also runs it on every push and pull request.
5. Open a pull request describing what changed and why, how it was tested, and on which platforms.

## The OpenSpec process

Requirements and decision history live in [`openspec/`](openspec/) ([OpenSpec](https://github.com/Fission-AI/OpenSpec)):

- `openspec/specs/` — current requirements with scenarios; change them together with the behavior;
- `openspec/changes/<name>/` — a change in progress: `proposal.md` (why), `design.md` (how, and why this way), `specs/` (requirement deltas), `tasks.md` (tasks);
- `openspec/changes/archive/` — completed changes;
- `openspec/config.yaml` — project context and rules for new changes.

Commands and skills for AI assistants (Claude Code, Cline, etc.) are not part of the repository — everyone has their own: `npm install -g @fission-ai/openspec` and `openspec init` generate them locally (`.claude/`, `.cline/`, `.clinerules/` are in `.gitignore`).

## Rules

- **Documentation in two languages.** If you change user-facing behavior or the build, update `docs/en/` and `docs/ru/` (and `README.md` / `README.ru.md`) in the same pull request. If you don't know one of the languages, write in yours and say so in the pull request — we'll finish the translation. `make test` checks that every document has its pair, a language switcher and working links.
- **UI strings are in Russian** until the app has interface translations. Code comments are in Russian.
- **Fyne widgets only in `internal/ui/...`.** Add a new `internal/` package to `internal/archtest/imports_test.go` and to [docs/en/architecture.md](docs/en/architecture.md) / [docs/ru/architecture.md](docs/ru/architecture.md).
- **The UI changes only on the main thread** — via `fyne.Do` from goroutines.
- **Platform code** goes through build tags and file suffixes, with a stub for the other platforms.
- **Third-party components.** If you change the Firefox ESR version (`tools/fetch-firefox`), GeckoView (`android/app/build.gradle.kts`) or modules in `go.mod`, update [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
- **Versions and releases** are handled by the maintainer: don't change `Version` in `FyneApp.toml` in a pull request.

## License

By contributing, you agree that your contributions are distributed under the project's license — [MIT](LICENSE).
