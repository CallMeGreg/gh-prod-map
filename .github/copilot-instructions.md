# General guidelines

- Use Go and the [cobra](https://github.com/spf13/cobra) library for command-line applications.
- Use [pterm](https://github.com/pterm/pterm) for all terminal output.
- Use [go-gh](https://github.com/cli/go-gh) for all GitHub API calls.
- Use [vhs](https://github.com/charmbracelet/vhs) to record demos.
- Keep reusable code in the `cmd/common.go` file.
- Always describe changes in a detailed plan before making them.
- Update the README file with any new features or changes.

# Naming conventions

- Use PascalCase for exported identifiers, interfaces, and type aliases.
- Use camelCase for unexported variables, functions, and methods.
- Use ALL_CAPS only for exported constants that represent fixed values.

# Error handling

- Return errors up the call stack and render them once at the top level (see `Root` in `cmd/root.go`).
- Always include contextual information in error messages.
- Use pterm for user-facing output instead of `fmt.Print*` or the `log` package.

# API preference

- Use [go-gh](https://github.com/cli/go-gh) for every GitHub API call.
- Always prefer the GraphQL API over the REST API when both expose the same data.
- When listing the organizations in an enterprise, always use the GraphQL API.
- Pass the `--hostname` value into the client so commands work against GitHub.com and GitHub Enterprise Server.
- If there is no confirmed API endpoint in the GitHub documentation, ask for one. Do NOT assume an endpoint.
