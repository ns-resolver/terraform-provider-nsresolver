# Every A record named "mail" in example.com.
data "nsresolver_records" "mail" {
  zone = "example.com"
  type = "A"
  name = "mail"
}

output "mail_addresses" {
  value = data.nsresolver_records.mail.records[*].answer
}
