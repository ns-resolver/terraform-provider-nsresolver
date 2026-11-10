# Terraform provider for NSResolver

Manage DNS zones, records and zone settings hosted on [NSResolver](https://nsresolver.com) with
Terraform, through the [NSResolver API](https://api.nsresolver.com/).

```hcl
terraform {
  required_providers {
    nsresolver = {
      source  = "ns-resolver/nsresolver"
      version = "~> 1.0"
    }
  }
}

provider "nsresolver" {} # token from NSRESOLVER_TOKEN

resource "nsresolver_zone" "example" {
  name = "example.com"
}

resource "nsresolver_record" "www" {
  zone   = nsresolver_zone.example.name
  type   = "A"
  name   = "www"
  answer = "203.0.113.10"
  ttl    = 300
}
```

Your API token is on your NSResolver account page. Full documentation is on the
[Terraform Registry](https://registry.terraform.io/providers/ns-resolver/nsresolver/latest/docs).

- Resources: `nsresolver_zone`, `nsresolver_record`
- Data sources: `nsresolver_zone`, `nsresolver_records`
- Import: zones by name (`example.com`), records by `<zone>/<record id>` (`example.com/1234`)

## Development

Requires Go (see `go.mod` for the version).

```sh
make build     # compile
make test      # unit tests (no network)
make docs      # regenerate docs/ from the schema and examples/ (commit the result)
```

### Acceptance tests

These run against the live API with a real account and create, change and delete zones under the
reserved `.test` TLD (accepted by NSResolver, never published). The account needs room for 2 zones
and about 20 records.

```sh
NSRESOLVER_TOKEN=nsr_... make testacc
```

### Releasing

1. Update `CHANGELOG.md`, commit.
2. Tag and push: `git tag v1.2.3 && git push origin v1.2.3`.
3. The Release workflow builds every platform, signs the checksums with the GPG key in the
   repository secrets (`GPG_PRIVATE_KEY`, `PASSPHRASE`) and publishes a GitHub release. The
   Terraform Registry picks it up within a few minutes.

## License

[MPL-2.0](LICENSE)
