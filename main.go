package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	redis "github.com/go-redis/redis/v7"
)

var (
	API_KEY         = getEnvDefault("API_KEY", "")
	REDIS_HOST_PORT = getEnvDefault("REDIS_HOST_PORT", "localhost:6379")
)

var rds = redis.NewClient(&redis.Options{
	Addr:     REDIS_HOST_PORT,
	Password: "", // no password set
	DB:       0,  // use default DB
})
var chanMap = make(map[string]chan bool)

type reCaptchaV2 struct {
	GoogleKey string `json:"googleKey"`
	PageUrl   string `json:"pageUrl"`
	Size      int    `json:"size"`
	Interval  int    `json:"interval"`
}

func main() {
	log.Println(API_KEY, REDIS_HOST_PORT)
	_, err := rds.Ping().Result()
	if err != nil {
		log.Fatal(err)
	}

	router := gin.Default()
	router.POST("/start", func(c *gin.Context) {
		var re reCaptchaV2
		err := c.BindJSON(&re)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		googleKey := re.GoogleKey
		pageUrl := re.PageUrl

		if googleKey == "" {
			c.String(http.StatusBadRequest, "missing googleKey")
			return
		}
		if pageUrl == "" {
			c.String(http.StatusBadRequest, "missing pageUrl")
			return
		}
		if re.Size <= 0 {
			c.String(http.StatusBadRequest, "missing size")
			return
		}
		if re.Interval <= 0 {
			c.String(http.StatusBadRequest, "missing interval")
			return
		}

		key := keyOf(googleKey, pageUrl)
		if _, ok := chanMap[key]; ok {
			c.String(http.StatusOK, "has been started")
			return
		}

		exit := make(chan bool, 1)
		chanMap[key] = exit
		go reCaptchaTask(googleKey, pageUrl, re.Size, re.Interval, exit)

		c.String(http.StatusOK, "started")
	})

	router.POST("/getOne", func(c *gin.Context) {
		var re reCaptchaV2
		err := c.BindJSON(&re)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		googleKey := re.GoogleKey
		pageUrl := re.PageUrl

		if googleKey == "" {
			c.String(http.StatusBadRequest, "missing googleKey")
			return
		}
		if pageUrl == "" {
			c.String(http.StatusBadRequest, "missing pageUrl")
			return
		}

		one := redisGetOne(googleKey, pageUrl)
		if one == "" {
			c.String(http.StatusAccepted, "not ready")
			return
		}
		c.String(http.StatusOK, one)
	})

	router.POST("/stop", func(c *gin.Context) {
		var re reCaptchaV2
		err := c.BindJSON(&re)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		googleKey := re.GoogleKey
		pageUrl := re.PageUrl

		if googleKey == "" {
			c.String(http.StatusBadRequest, "missing googleKey")
			return
		}
		if pageUrl == "" {
			c.String(http.StatusBadRequest, "missing pageUrl")
			return
		}

		key := keyOf(googleKey, pageUrl)
		if ch, ok := chanMap[key]; !ok {
			c.String(http.StatusNotFound, "not found")
		} else {
			ch <- true
			c.String(http.StatusOK, "stopped")
		}
	})

	router.Run()
}

func keyOf(googleKey, pageUrl string) string {
	return fmt.Sprintf("rechaptcha:%s:%s", googleKey, pageUrl)
}

func keyInNS(k string) string {
	return fmt.Sprintf("rechaptcha:%s", k)
}

func redisAdd(googleKey, pageUrl, response string) {
	rds.SAdd(keyOf(googleKey, pageUrl), response)
	rds.Set(keyInNS(response), "1", 110*time.Second)
}

func redisGetOne(googleKey, pageUrl string) string {
	for {
		response, err := rds.SPop(keyOf(googleKey, pageUrl)).Result()
		if err != nil {
			log.Println(err)
			return ""
		}

		if v, err := rds.Get(keyInNS(response)).Result(); err != nil {
			log.Println(err)
			return ""
		} else if v != "" {
			rds.Del(keyInNS(response))
			return response
		}
	}
}

func reCaptchaResult(googleKey, pageUrl, captchaID string) string {
	n := 0
	for {
		n++
		log.Printf("查看进度 %v %v", captchaID, n)

		res, err := http.Get(fmt.Sprintf("http://2captcha.com/res.php?key=%s&action=get&id=%s", API_KEY, captchaID))
		if err != nil {
			log.Println("get:", err)
			if res.Body != nil {
				res.Body.Close()
			}
			goto wait
		}

		if res.Body != nil {
			body2 := readBody(res)
			if strings.Contains(body2, "CAPCHA_NOT_READY") {
				goto wait
			}
			reCaptchaResponse := strings.SplitN(body2, "|", 2)[1]
			redisAdd(googleKey, pageUrl, reCaptchaResponse)
			return reCaptchaResponse
		}

	wait:
		time.Sleep(2)
	}
}

func reCaptchaTask(googleKey, pageUrl string, size, interval int, exit <-chan bool) {
	tasks := make(chan int, size)

	newTask := func() {
		defer func() { <-tasks }()

		res, err := http.Post(fmt.Sprintf("http://2captcha.com/in.php?key=%s&method=userrecaptcha&googlekey=%s&pageurl=%s", API_KEY, googleKey, pageUrl), "plain/text", nil)
		if err != nil {
			log.Println(err)
			return
		}
		if res.Body != nil {
			body := readBody(res)
			if body == "ERROR_ZERO_BALANCE" {
				log.Fatal("ERROR_ZERO_BALANCE")
			}
			captchaID := strings.SplitN(body, "|", 2)[1]
			log.Println(captchaID)

			if captchaID == "" {
				return
			}
			reCaptchaResult(googleKey, pageUrl, captchaID)
		}
	}

	for {
		select {
		case tasks <- 1:
			go newTask()
			time.Sleep(10 * time.Second)
		default:
			select {
			case <-exit:
				log.Println("stop creating new task")
				delete(chanMap, keyOf(googleKey, pageUrl))
				return
			default:
				time.Sleep(time.Duration(interval) * time.Second)
				continue
			}
		}
	}
}

func readBody(res *http.Response) string {
	body := res.Body
	if body == nil {
		return ""
	}

	defer body.Close()
	bodyBytes, err := ioutil.ReadAll(body)
	if err != nil {
		log.Fatal(err)
	}
	return string(bodyBytes)
}

func getEnvDefault(k, dft string) string {
	v := os.Getenv(k)
	if v == "" {
		return dft
	}
	return v
}
