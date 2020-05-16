build-alpine-image:
	GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o 2captcha-pool -v main.go
	docker build -t weaming/2captcha-pool .
