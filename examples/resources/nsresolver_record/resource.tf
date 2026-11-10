resource "nsresolver_record" "apex" {
  zone   = nsresolver_zone.example.name
  type   = "A"
  name   = "@"
  answer = "203.0.113.10"
  ttl    = 300
}

# Two A records for one name: one resource each.
resource "nsresolver_record" "mail_1" {
  zone   = nsresolver_zone.example.name
  type   = "A"
  name   = "mail"
  answer = "203.0.113.20"
}

resource "nsresolver_record" "mail_2" {
  zone   = nsresolver_zone.example.name
  type   = "A"
  name   = "mail"
  answer = "203.0.113.21"
}

resource "nsresolver_record" "www" {
  zone   = nsresolver_zone.example.name
  type   = "CNAME"
  name   = "www"
  answer = "example.com"
}

resource "nsresolver_record" "mx" {
  zone     = nsresolver_zone.example.name
  type     = "MX"
  name     = "@"
  answer   = "mail.example.com"
  priority = 10
}

resource "nsresolver_record" "spf" {
  zone   = nsresolver_zone.example.name
  type   = "TXT"
  name   = "@"
  answer = "v=spf1 mx -all"
}

resource "nsresolver_record" "sip" {
  zone     = nsresolver_zone.example.name
  type     = "SRV"
  name     = "_sip._tcp"
  answer   = "sip.example.com"
  priority = 10
  weight   = 5
  port     = 5060
}

resource "nsresolver_record" "caa" {
  zone   = nsresolver_zone.example.name
  type   = "CAA"
  name   = "@"
  tag    = "issue"
  answer = "letsencrypt.org"
}

# Delegate a subdomain to other nameservers.
resource "nsresolver_record" "dev_ns" {
  zone   = nsresolver_zone.example.name
  type   = "NS"
  name   = "dev"
  answer = "ns1.other-provider.net"
}
