build-alpine-image:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o 2captcha-pool -v *.go
	docker build -t weaming/2captcha-pool .
