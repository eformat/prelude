.PHONY: client-run server-run build-client build-server build-all run-all podman-server-build podman-client-build podman-build-all podman-push-all helm-deploy

build-all: build-server build-client

run-all:
	$(MAKE) server-run &
	$(MAKE) client-run

client-run:
	cd client && npx next dev -H 0.0.0.0 -p 3000

server-run:
	cd server && ./server

build-client:
	cd client && npm run build

build-server:
	cd server && go build ./...

podman-build-all: podman-server-build podman-client-build

podman-server-build:
	podman build $(PODMAN_ARGS) -f Containerfile.server -t quay.io/eformat/prelude-server:latest .

podman-client-build:
	podman build $(PODMAN_ARGS) -f Containerfile.client -t quay.io/eformat/prelude-client:latest .

podman-push-all:
	podman push quay.io/eformat/prelude-server:latest
	podman push quay.io/eformat/prelude-client:latest

helm-deploy:
	helm upgrade --install prelude ./chart $(HELM_ARGS)
