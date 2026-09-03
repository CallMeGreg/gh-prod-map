# gh-extension-template

A template repository for building **precompiled [GitHub CLI](https://cli.github.com) extensions in Go**.

It is scaffolded with `gh extension create --precompiled=go` and wired up with a preferred, opinionated set of libraries:

| Concern | Library |
| ------- | ------- |
| Subcommands & flags | [spf13/cobra](https://github.com/spf13/cobra) |
| Terminal UI (spinners, progress bars, tables) | [pterm/pterm](https://github.com/pterm/pterm) |
| GitHub API calls (GraphQL preferred) | [cli/go-gh](https://github.com/cli/go-gh) |
| Demo recordings | [charmbracelet/vhs](https://github.com/charmbracelet/vhs) |

Two example subcommands (`orgs` and `repos`) are included so you can see the whole stack working end to end. Replace them with your own.

![demo](demo/demo.gif)

## Use this template

Click **Use this template** on GitHub, or create a repo from the CLI:

```sh
gh repo create <owner>/gh-my-extension --template CallMeGreg/gh-extension-template --private --clone
```

### Rename the placeholders

After creating your repository, replace the template name in a few places:

1. `go.mod` — the module path `github.com/CallMeGreg/gh-extension-template`
2. `main.go` — the `cmd` import path
3. `cmd/root.go` — the `Use:` field on `RootCmd`
4. `.gitignore` — the `/gh-extension-template` binary entries
5. `demo/demo.tape` — the `gh extension-template` invocations and the `Output` path
6. `SECURITY.md` and `.github/CODEOWNERS` — the owner/repo references

A find/replace of `extension-template` (and the `CallMeGreg` owner) covers the code; then swap the example `orgs`/`repos` commands for your own.

## Prerequisites

- [Go](https://go.dev/dl/) — see the version pinned in `go.mod`
- [GitHub CLI](https://cli.github.com) (`gh`), authenticated with `gh auth login`

## Build and run locally

```sh
go build -o gh-extension-template .
gh extension install .
gh extension-template --help
```

While iterating you can also run without installing:

```sh
go run . repos --org cli --limit 5
```

## Example commands

### List the organizations in an enterprise (GraphQL)

```sh
gh extension-template orgs --enterprise <enterprise-slug>
```

Listing the organizations in an enterprise is only supported by the GraphQL API, so this command always uses GraphQL.

### List the repositories in an organization (GraphQL)

```sh
gh extension-template repos --org cli --limit 5
```

```
 INFO  Repositories in organization "cli": 5

Name                    | Visibility | Language | Stars
browser                 | PUBLIC     | Go       | 98
cli                     | PUBLIC     | Go       | 46042
gh-extension-precompile | PUBLIC     | Shell    | 128
gh-webhook              | PUBLIC     | Go       | 42
go-gh                   | PUBLIC     | Go       | 435
```

### Global flags

| Flag | Alias | Default | Description |
| ---- | ----- | ------- | ----------- |
| `--enterprise` | `-e` | | GitHub Enterprise slug (mutually exclusive with `--org`) |
| `--org` | `-o` | | GitHub organization login (mutually exclusive with `--enterprise`) |
| `--hostname` | `-u` | `github.com` | GitHub host (e.g. `github.example.com` for GitHub Enterprise Server) |
| `--limit` | `-L` | `30` | Maximum number of results to fetch |

## Project structure

```
.
├── main.go                # Entry point; calls cmd.Root()
├── cmd/
│   ├── root.go            # Root command + persistent flags
│   ├── common.go          # Reusable go-gh clients, pterm helpers, GraphQL queries
│   ├── orgs.go            # `orgs` subcommand
│   └── repos.go           # `repos` subcommand
├── demo/
│   └── demo.tape          # VHS script for the README demo
├── .github/
│   ├── workflows/
│   │   ├── release.yml            # Auto-versioned precompiled releases
│   │   └── dependency-review.yml  # Blocks vulnerable dependencies on PRs
│   ├── instructions/              # Language-specific guidance for Copilot
│   ├── copilot-instructions.md    # Repo-wide guidance for Copilot
│   ├── dependabot.yml             # Weekly GitHub Actions updates
│   ├── pull_request_template.md   # Drives the release type
│   └── CODEOWNERS
├── SECURITY.md
├── LICENSE
└── go.mod
```

## API usage philosophy

- Every GitHub API call goes through [cli/go-gh](https://github.com/cli/go-gh).
- **Prefer the GraphQL API over REST** whenever both expose the same data.
- Listing the organizations in an enterprise **always uses GraphQL** (REST cannot do it).
- Both a GraphQL and a REST client helper live in `cmd/common.go`; commands pass the `--hostname` value so they work against GitHub.com and GitHub Enterprise Server.

## Recording demos with VHS

The demo GIF is generated from [`demo/demo.tape`](demo/demo.tape) with [VHS](https://github.com/charmbracelet/vhs):

```sh
go build -o gh-extension-template .
gh extension install .
vhs demo/demo.tape
```

Edit the tape to script whatever commands best show off your extension.

## Releasing

Releases are automated by [`.github/workflows/release.yml`](.github/workflows/release.yml):

1. Open a PR and check a box in the **Release Type** section of the PR description (Major / Minor / Patch).
2. When the PR merges to `main`, the workflow reads that box, computes the next semantic version, tags it, and runs [`cli/gh-extension-precompile`](https://github.com/cli/gh-extension-precompile) to build cross-platform binaries with build provenance attestations.
3. Dependabot PRs default to a patch release.

> **Note:** The first push to `main` (creating the repo from this template) has no PR, so it defaults to a patch release and cuts `v0.0.1` automatically.

Users can then install your extension with `gh extension install <owner>/gh-my-extension`.

## License

[MIT](LICENSE)
