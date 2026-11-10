resource "nsresolver_zone" "example" {
  name = "example.com"

  # Optional; left out, NSResolver's defaults are used.
  default_ttl     = 3600
  negative_answer = 300
}
