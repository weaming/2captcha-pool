package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	API_KEY = getEnvDefault("API_KEY", "")
)

var siteMap = make(map[string]*Site)
var lock = sync.RWMutex{}

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

func (s *Site) Stop() {
	s.Lock()
	defer s.Unlock()
	s.idle = true
	s.stop <- true
}

func main() {
	log.Println(API_KEY)

	router := gin.Default()
	router.POST("/getOne", func(c *gin.Context) {
		task := &Task{}
		err := c.BindJSON(task)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		googleKey := task.GoogleKey
		pageURL := task.PageURL

		if googleKey == "" {
			c.String(http.StatusBadRequest, "missing googleKey")
			return
		}
		if pageURL == "" {
			c.String(http.StatusBadRequest, "missing pageUrl")
			return
		}
		if task.Size == 0 {
			c.String(http.StatusBadRequest, "missing size")
			return
		}
		if task.Interval == 0 {
			c.String(http.StatusBadRequest, "missing interval")
			return
		}
		if task.Lives == 0 {
			c.String(http.StatusBadRequest, "missing lives")
			return
		}

		v := getOne(task)
		if v == nil {
			c.String(http.StatusAccepted, "not ready")
			return
		}
		c.JSON(http.StatusOK, v)
	})

	router.Run()
}

func getOne(task *Task) interface{} {
	lock.Lock()
	defer lock.Unlock()

	key := fmt.Sprintf("%s:%s", task.PageURL, task.GoogleKey)
	if site, ok := siteMap[key]; ok {
		site.task = task
		if site.idle {
			site.idle = false
			go reCaptchaTask(site)
		}
		return getFromSite(site)
	} else {
		site := &Site{
			task: task,
			ids:  make(chan string, task.Size*10),
			stop: make(chan bool, 1),
			idle: false,
		}
		fn := func(cache *Cache, key string) {
			site.task.Lives--
			if site.task.Lives <= 0 {
				site.Stop()
			}
		}
		site.results = NewCache(&fn)
		siteMap[key] = site
		go reCaptchaTask(site)
		return nil
	}
}

func getFromSite(site *Site) interface{} {
begin:
	select {
	case id := <-site.ids:
		v, ok := site.results.Get(id)
		if ok {
			return v
		}
		goto begin
	default:
		return nil
	}
}

func addToSite(site *Site, captchaID, reCaptchaResponse string) {
	site.ids <- captchaID
	site.results.Set(captchaID, reCaptchaResponse, 100*time.Second)
}

func reCaptchaResult(site *Site, captchaID string) {
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
			arr := strings.SplitN(body2, "|", 2)
			if len(arr) < 2 {
				log.Println(body2)
				if body2 == "ERROR_CAPTCHA_UNSOLVABLE" {
					return
				}
				goto wait
			}
			reCaptchaResponse := arr[1]
			addToSite(site, captchaID, reCaptchaResponse)
			return
		}

	wait:
		time.Sleep(2)
	}
}

func reCaptchaTask(site *Site) {
	tasks := make(chan int, site.task.Size)

	newTask := func() {
		defer func() { <-tasks }()

		url := fmt.Sprintf("http://2captcha.com/in.php?key=%s&method=userrecaptcha&googlekey=%s&pageurl=%s", API_KEY, site.task.GoogleKey, site.task.PageURL)
		res, err := http.Post(url, "plain/text", nil)
		if err != nil {
			log.Println(err)
			return
		}
		if res.Body != nil {
			body := readBody(res)
			log.Println(body)
			if body == "ERROR_ZERO_BALANCE" {
				log.Fatal("ERROR_ZERO_BALANCE")
			}
			captchaID := strings.SplitN(body, "|", 2)[1]
			log.Println(captchaID)

			if captchaID == "" {
				return
			}
			reCaptchaResult(site, captchaID)
		}
	}

	for {
		select {
		case tasks <- 1:
			go newTask()
			time.Sleep(time.Duration(site.task.Interval) * time.Second)
		case <-site.stop:
			log.Println("stop creating new task")
			return
		default:
			time.Sleep(3 * time.Second)
			continue
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
