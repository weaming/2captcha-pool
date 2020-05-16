FROM alpine:latest
WORKDIR /app
EXPOSE 8080
COPY 2captcha-pool .
CMD ["./2captcha-pool"]
