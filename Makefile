build:
	go build -v -o bin/ ./...

test:
	go test -v ./...


seed:
	docker build -t gedcb/seed .
	kubectl apply -f config/kubernetes/seed.yaml

reseed:
	kubectl rollout restart statefulset seed