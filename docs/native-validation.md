# Native application validation

Native validation uses Go 1.27.1 and MyGo v0.6.2. Release-specific changes are recorded under `docs/releases/`.

| Requirement | Evidence |
| --- | --- |
| Native Go interface and editor | `main.go` uses `Content: ui.View`; `internal/app/view.go` uses `ui/editor`. A packaged Windows process rendered and captured its native window. |
| Light/dark colors | Palette and editor-color tests and rendered light/dark snapshots pass. |
| Generated icon | Built-in image generation produced `resources/icon.png`; transparent corners and square dimensions were checked. Header, window and all four packaged resource copies use that same PNG. Prompt and provenance are in `icon-generation.md`. |
| GitHub list and file operations | HTTP fixture tests exercise authentication, pagination, content retrieval, create, patch/rename/delete-file and delete-Gist. Native interaction tests exercise the corresponding controls and resulting state. |
| Editor behavior | Native UI tests verify read-only default, Chinese text editing, save/cancel, unsaved-change confirmation, Raw actions, CRLF preservation, search and minimum-size geometry. Language coverage includes PowerShell, Vue and CSS variants. |
| Secure storage | Password, lock and fallback tests pass. A separate, temporary Windows Credential Manager entry passed save/load/delete integration and was deleted. |
| Windows Hello / Touch ID | Windows Hello availability check returned true without prompting. Password fallback and biometric success/cancellation are tested with an injected provider. Darwin LocalAuthentication code builds for Intel and Apple Silicon. Physical biometric verification was not performed. |
| Formal releases | Tests verify checks against the latest formal release, installer-only releases, version comparisons, rejection of drafts and prereleases, and migration from obsolete update settings. Signed-update validation and archive traversal tests remain covered independently. |
| Native packaging | The v1.0.0 Windows amd64 executable, NSIS installer and portable archive are built and checked locally. CI builds Windows amd64/arm64 and macOS Intel/Apple Silicon installers and portable archives. |
| Release workflow | Actionlint validates the workflow. Tests run on Windows, macOS Intel and macOS Apple Silicon. Only formal release tags matching `mygo.json` compile and publish installation packages. All eight installer/ZIP assets must be present before publishing, with a combined SHA-256 checksum file. The workflow requires no custom secret. |
| macOS validation in CI | The native runner executes the bundled binary with `--version`, verifies its Info.plist version, checks its CPU architecture with `lipo` and verifies its bundle signature with `codesign`. DMG creation runs on the matching Mac runner. |

Final checks passed:

```text
go test ./... -timeout 120s
go vet ./...
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -shellcheck= .github/workflows/native.yml
git -c core.safecrlf=false diff --check
```

Installer-only packaging is tested without a private key or public-key file. A stale automatic-update manifest must be removed when building a portable archive. Fixtures do not read an existing account token or modify real GitHub data.

Physical biometric verification and destructive operations on a live GitHub account are outside the automated test coverage. The Mac runner checks the packaged command-line entry point and ad-hoc signature; it does not exercise the graphical interface or Touch ID. The release workflow does not perform Apple notarization.
