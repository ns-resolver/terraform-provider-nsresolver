default: build

build:
	go build -v ./...

install:
	go install -v ./...

test:
	go test -v -count=1 ./...

# Acceptance tests against the live API: NSRESOLVER_TOKEN=... make testacc
# Uses OpenTofu if it's installed and Terraform isn't (the test framework needs one of them).
TOFU := $(shell command -v terraform >/dev/null 2>&1 || command -v tofu 2>/dev/null)
testacc:
	TF_ACC=1 $(if $(TOFU),TF_ACC_TERRAFORM_PATH=$(TOFU) TF_ACC_PROVIDER_HOST=registry.opentofu.org TF_ACC_PROVIDER_NAMESPACE=hashicorp) \
		go test -v -count=1 -timeout 30m ./...

docs:
	go generate ./...

.PHONY: default build install test testacc docs
