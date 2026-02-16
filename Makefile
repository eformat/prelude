.PHONY: client-run server-run cluster-claimer-run cluster-authenticator-run build-client build-server build-cluster-claimer build-cluster-authenticator build-all run-all podman-server-build podman-client-build podman-cluster-claimer-build podman-cluster-authenticator-build podman-build-all podman-push-all helm-deploy

build-all: build-server build-cluster-claimer build-cluster-authenticator build-client

run-all:
	$(MAKE) server-run &
	$(MAKE) client-run

client-run:
	cd client && npx next dev -H 0.0.0.0 -p 3000

server-run:
	cd server && ./server

cluster-claimer-run:
	cd cluster-claimer && ./cluster-claimer

build-client:
	cd client && NEXT_OUTPUT=standalone npm run build

build-server:
	cd server && go build ./...

build-cluster-claimer:
	cd cluster-claimer && go build ./...

build-cluster-authenticator:
	cd cluster-authenticator && go build ./...

cluster-authenticator-run:
	cd cluster-authenticator && ./cluster-authenticator

podman-build-all: podman-server-build podman-cluster-claimer-build podman-cluster-authenticator-build podman-client-build

podman-server-build:
	podman build $(PODMAN_ARGS) -f Containerfile.server -t quay.io/eformat/prelude-server:latest .

podman-client-build:
	podman build $(PODMAN_ARGS) -f Containerfile.client -t quay.io/eformat/prelude-client:latest .

podman-cluster-claimer-build:
	podman build $(PODMAN_ARGS) -f Containerfile.cluster-claimer -t quay.io/eformat/prelude-cluster-claimer:latest .

podman-cluster-authenticator-build:
	podman build $(PODMAN_ARGS) -f Containerfile.cluster-authenticator -t quay.io/eformat/prelude-cluster-authenticator:latest .

podman-push-all:
	podman push quay.io/eformat/prelude-server:latest
	podman push quay.io/eformat/prelude-cluster-claimer:latest
	podman push quay.io/eformat/prelude-cluster-authenticator:latest
	podman push quay.io/eformat/prelude-client:latest

helm-deploy:
	helm upgrade --install prelude ./chart $(HELM_ARGS)
