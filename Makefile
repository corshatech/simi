# Copyright Corsha Inc. All Rights Reserved.
# SPDX-License-Identifier: Apache-2.0

REGISTRY ?=
TAG ?= latest
PLATFORM ?= linux/amd64
IMAGE_PREFIX = $(if $(REGISTRY),$(REGISTRY)/,)

all: test simi build-simi-image

lint: lint-go lint-shell lint-helm

lint-go:
	golangci-lint run --timeout 2m

lint-shell:
	shellcheck scripts/*.sh

lint-helm:
	helm lint k8s/helm/simi --values k8s/helm/simi/ci/ci-values.yaml -f k8s/helm/simi/values.yaml

test:
	GO111MODULE=on go test -race -v -coverprofile=coverage.out ./...

tidy:
	GO111MODULE=on go mod tidy

simi:
	GOOS=linux GOARCH=amd64 go build -o bin/simi cmd/simi.go
	GOOS=linux GOARCH=amd64 go build -o benchmark/bench-consumer benchmark/cmd/consumer.go

simi-image:
	docker build -t simi-test:latest bin

consumer-image:
	docker build -t consumer-test:latest benchmark

build-images:
	docker build -t $(IMAGE_PREFIX)corsha/simi-robot:$(TAG) --platform $(PLATFORM) bin
	docker build -t $(IMAGE_PREFIX)corsha/simi-consumer:$(TAG) --platform $(PLATFORM) benchmark
	docker build --ssh default -t $(IMAGE_PREFIX)corsha/fabric-chaincode:$(TAG) -f fabric-chaincode/Dockerfile --platform $(PLATFORM) ./fabric-chaincode

publish-images: build-images
	@if [ -z "$(REGISTRY)" ]; then \
		echo "ERROR: REGISTRY variable must be set to publish images. Example: REGISTRY=myregistry.com make publish-images"; \
		exit 1; \
	fi
	docker push $(IMAGE_PREFIX)corsha/simi-robot:$(TAG)
	docker push $(IMAGE_PREFIX)corsha/simi-consumer:$(TAG)
	docker push $(IMAGE_PREFIX)corsha/fabric-chaincode:$(TAG)

.PHONY: tidy lint test simi build-simi-image build-images publish-images
