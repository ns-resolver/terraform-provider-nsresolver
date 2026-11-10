data "nsresolver_zone" "example" {
  name = "example.com"
}

output "example_status" {
  value = data.nsresolver_zone.example.status
}
