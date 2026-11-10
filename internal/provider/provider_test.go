package provider

import (
	"fmt"
	"math/rand"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Acceptance tests run against the live API with a real account: TF_ACC=1 NSRESOLVER_TOKEN=...
// They create zones under the reserved .test TLD, which NSResolver accepts but never publishes,
// and delete everything they create. The account needs room for 2 zones and about 20 records.

var testProviders = map[string]func() (tfprotov6.ProviderServer, error){
	"nsresolver": providerserver.NewProtocol6WithError(New("test")()),
}

func preCheck(t *testing.T) {
	if os.Getenv("NSRESOLVER_TOKEN") == "" {
		t.Fatal("NSRESOLVER_TOKEN must be set for acceptance tests")
	}
}

func testZone() string { return fmt.Sprintf("tfacc-%06d.test", rand.Intn(1000000)) }

func TestAccZone(t *testing.T) {
	zone := testZone()
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`resource "nsresolver_zone" "z" { name = %q }`, zone),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsresolver_zone.z", "id", zone),
					resource.TestCheckResourceAttr("nsresolver_zone.z", "refresh", "7200"),
					resource.TestCheckResourceAttr("nsresolver_zone.z", "negative_answer", "300"),
					resource.TestCheckResourceAttrSet("nsresolver_zone.z", "status"),
				),
			},
			{
				// Lowering refresh and retry together only works because they're sent together.
				Config: fmt.Sprintf(`resource "nsresolver_zone" "z" {
  name            = %q
  refresh         = 1800
  retry           = 900
  negative_answer = 600
}`, zone),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsresolver_zone.z", "refresh", "1800"),
					resource.TestCheckResourceAttr("nsresolver_zone.z", "retry", "900"),
					resource.TestCheckResourceAttr("nsresolver_zone.z", "negative_answer", "600"),
					resource.TestCheckResourceAttr("nsresolver_zone.z", "default_ttl", "3600"),
				),
			},
			{ResourceName: "nsresolver_zone.z", ImportState: true, ImportStateId: zone, ImportStateVerify: true},
		},
	})
}

func TestAccZoneWithSettings(t *testing.T) {
	zone := testZone()
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`resource "nsresolver_zone" "z" {
  name        = "%s."
  default_ttl = 300
  expire      = 604800
}`, zone),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("nsresolver_zone.z", "name", zone+"."),
				resource.TestCheckResourceAttr("nsresolver_zone.z", "id", zone),
				resource.TestCheckResourceAttr("nsresolver_zone.z", "default_ttl", "300"),
				resource.TestCheckResourceAttr("nsresolver_zone.z", "expire", "604800"),
			),
		}},
	})
}

func TestAccRecords(t *testing.T) {
	zone := testZone()
	config := func(mailIP, txt string) string {
		return fmt.Sprintf(`
resource "nsresolver_zone" "z" { name = %[1]q }

resource "nsresolver_record" "mail1" {
  zone   = nsresolver_zone.z.name
  type   = "A"
  name   = "mail"
  answer = %[2]q
  ttl    = 300
}

resource "nsresolver_record" "mail2" {
  zone   = nsresolver_zone.z.name
  type   = "A"
  name   = "mail.%[1]s"
  answer = "203.0.113.11"
}

resource "nsresolver_record" "v6" {
  zone   = nsresolver_zone.z.name
  type   = "AAAA"
  name   = "@"
  answer = "2001:0DB8:0000::1"
}

resource "nsresolver_record" "mx" {
  zone     = nsresolver_zone.z.name
  type     = "MX"
  name     = "@"
  answer   = "Mail.%[1]s."
  priority = 10
}

resource "nsresolver_record" "txt" {
  zone   = nsresolver_zone.z.name
  type   = "TXT"
  name   = "@"
  answer = %[3]q
}

resource "nsresolver_record" "srv" {
  zone     = nsresolver_zone.z.name
  type     = "SRV"
  name     = "_sip._tcp"
  answer   = "sip.example.net"
  priority = 10
  weight   = 5
  port     = 5060
}

resource "nsresolver_record" "caa" {
  zone   = nsresolver_zone.z.name
  type   = "CAA"
  name   = "@"
  answer = "letsencrypt.org;validationmethods=dns-01"
  tag    = "issue"
}

resource "nsresolver_record" "www" {
  zone   = nsresolver_zone.z.name
  type   = "CNAME"
  name   = "www"
  answer = "@"
}

data "nsresolver_records" "mail" {
  zone       = nsresolver_zone.z.name
  type       = "A"
  name       = "mail"
  depends_on = [nsresolver_record.mail1, nsresolver_record.mail2]
}
`, zone, mailIP, txt)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{
				// Non-canonical spellings (full names, trailing dots, long IPv6, quoted TXT, "@" targets)
				// must apply cleanly and then plan empty (checked by the test framework).
				Config: config("203.0.113.10", `"v=spf1 -all"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsresolver_record.mail1", "fqdn", "mail."+zone),
					resource.TestCheckResourceAttr("nsresolver_record.mail2", "ttl", "3600"),
					resource.TestCheckResourceAttr("nsresolver_record.caa", "flags", "0"),
					resource.TestCheckResourceAttr("data.nsresolver_records.mail", "records.#", "2"),
				),
			},
			{
				Config: config("203.0.113.20", "v=spf1 include:example.net -all"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsresolver_record.mail1", "answer", "203.0.113.20"),
					resource.TestCheckResourceAttr("nsresolver_record.txt", "answer", "v=spf1 include:example.net -all"),
				),
			},
			{
				ResourceName: "nsresolver_record.srv", ImportState: true, ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					r := s.RootModule().Resources["nsresolver_record.srv"]
					return zone + "/" + r.Primary.ID, nil
				},
			},
		},
	})
}
