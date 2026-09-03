---
applyTo: '**/*.go'
---

# Writing GraphQL queries

Prefer the GraphQL API over REST whenever both expose the same data. When
listing the organizations in an enterprise, the GraphQL API is required.

# Paginate with a cursor

Request one page at a time and follow `pageInfo.endCursor` until
`pageInfo.hasNextPage` is `false`. Declare the cursor as a nullable `String`
variable that starts as `null`:

```graphql
query($slug: String!, $first: Int!, $endCursor: String) {
  enterprise(slug: $slug) {
    organizations(first: $first, after: $endCursor, orderBy: {field: LOGIN, direction: ASC}) {
      nodes {
        login
        name
      }
      pageInfo {
        hasNextPage
        endCursor
      }
    }
  }
}
```

In Go, pass the cursor as a `*string` that is `nil` on the first request:

```go
variables := map[string]interface{}{
    "slug":      enterprise,
    "first":     100,
    "endCursor": endCursor, // *string, nil for the first page
}
```

See `ListEnterpriseOrganizations` and `ListOrganizationRepositories` in
`cmd/common.go` for complete, paginated examples.

# Rate limits

A GraphQL response can return HTTP 200 while still including an `errors` array;
go-gh surfaces these as a Go error, so always check the error returned by
`client.Do`. If you exceed the primary rate limit, wait until the time in the
`x-ratelimit-reset` header before retrying.
