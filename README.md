# 2captcha-pool

[2captcha.com](https://2captcha.com) 's result pool

```shell
docker run -it --rm -e REDIS_HOST_PORT=host.docker.internal:6379 -e API_KEY=<your-2captcha-key> weaming/2captcha-pool
```

```go
API_KEY         = getEnvDefault("API_KEY", "")
REDIS_HOST_PORT = getEnvDefault("REDIS_HOST_PORT", "localhost:6379")

type reCaptchaV2 struct {
	GoogleKey string `json:"googleKey"`
	PageUrl   string `json:"pageUrl"`
	Size      int    `json:"size"`
	Interval  int    `json:"interval"`
}
```
