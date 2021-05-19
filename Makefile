build:
	go build

build-image: build
	docker build -t quay.io/surajd/k8s-secret-access-validator .

push-image: build-image
	docker push quay.io/surajd/k8s-secret-access-validator
