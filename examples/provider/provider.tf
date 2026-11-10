terraform {
  required_providers {
    nsresolver = {
      source  = "ns-resolver/nsresolver"
      version = "~> 1.0"
    }
  }
}

# The token is on your NSResolver account page. Better than writing it here:
# export NSRESOLVER_TOKEN=nsr_...
provider "nsresolver" {}
