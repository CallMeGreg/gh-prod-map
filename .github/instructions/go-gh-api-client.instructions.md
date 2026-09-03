---
applyTo: '**/*.go'
---

# Use go-gh for GitHub API calls

Create API clients with the helpers in `cmd/common.go`:

- `NewGraphQLClient(hostname)` — preferred for all data the GraphQL API exposes.
- `NewRESTClient(hostname)` — only when the GraphQL API cannot return the data.

Pass the `--hostname` flag value as `hostname` so the extension works against
both GitHub.com and GitHub Enterprise Server.

# GraphQL example (cli/go-gh)

```go
client, err := NewGraphQLClient("github.com")
if err != nil {
    return err
}

query := `query($login: String!) {
    organization(login: $login) {
        name
    }
}`
variables := map[string]interface{}{"login": "github"}

var response struct {
    Organization struct {
        Name string
    }
}
if err := client.Do(query, variables, &response); err != nil {
    return err
}
```

`GraphQLClient.Do` unmarshals the response's `data` field into the provided
struct and returns any GraphQL errors as a Go error.

# REST example (cli/go-gh)

Only reach for REST when GraphQL cannot return the data you need.

```go
client, err := NewRESTClient("github.com")
if err != nil {
    return err
}

response := []struct{ Name string }{}
if err := client.Get("repos/cli/cli/tags", &response); err != nil {
    return err
}
```
