version = 0.0.1

.PHONY: run build install image-tunnel image-dns image-template

run:
	go run -ldflags "-X main.version=$(version)" .

build:
	go build -ldflags "-X main.version=$(version)" .

install:
	go build -ldflags "-X main.version=$(version)" -o /usr/local/bin/loop .

images: image-tunnel image-dns

image-code:
	docker build helpers/loop-code --tag adrianliechti/loop-code --platform linux/amd64 && \
	docker push adrianliechti/loop-code

image-dns:
	docker build helpers/loop-tunnel --tag adrianliechti/loop-tunnel --platform linux/amd64 && \
	docker push adrianliechti/loop-tunnel

image-tunnel:
	docker build helpers/loop-dns --tag adrianliechti/loop-dns --platform linux/amd64 && \
	docker push adrianliechti/loop-dns

image-template: helpers/loop-template/*
	for path in $^ ; do \
		tag=$$(basename $$path) ; \
		docker build $$path --tag adrianliechti/loop-template:$$tag --platform linux/amd64 ; \
		docker push adrianliechti/loop-template:$$tag ; \
	done