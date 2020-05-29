# 2captcha-pool

[2captcha.com](https://2captcha.com) 's result pool

```shell
docker run -it --rm -e REDIS_HOST_PORT=host.docker.internal:6379 -e API_KEY=<your-2captcha-key> weaming/2captcha-pool
```

```go
API_KEY         = getEnvDefault("API_KEY", "")

type Task struct {
	GoogleKey string `json:"googleKey"`
	PageURL   string `json:"pageUrl"`
	Size      int    `json:"size"`
	Interval  int    `json:"interval"`
	Lives     int    `json:"lives"` // after n token unused, stop loop
}

type Site struct {
	sync.Mutex // lock for idle state
	task       *Task
	ids        chan string
	results    *Cache
	stop       chan bool
	idle       bool
}
```

## API

* POST `/getOne`: get one [`response` result](https://developers.google.com/recaptcha/docs/verify#api_request). Start or stop task automatically

```sh
curl https://2captcha-pool.drink.cafe/getOne \
    -d '{"googleKey": "6LerB_cSAAAAACHfjoc7wuQ28ssaqm2mEZN02s3d", "pageUrl": "https://www.google.com/recaptcha/api2/demo", "size": 2, "interval": 10, "lives": 1}'
```
