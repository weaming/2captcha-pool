package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

var client = &http.Client{
	Transport: &http.Transport{
		MaxIdleConnsPerHost: 1024,
		MaxIdleConns:        0,
	},
	Timeout: 10 * time.Second,
}

func keyOfTask(task *Task) string {
	return fmt.Sprintf("%s:%s:%v", task.PageURL, task.GoogleKey, task.NoLimit)
}

func GetOneCrackResult(task *Task) interface{} {
	lock.Lock()
	defer lock.Unlock()

	key := keyOfTask(task)
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
		afterExpire := func(cache *Cache, key string) {
			if site.task.NoLimit {
				site.task.Lives = 1 // 永不休眠
			} else {
				site.task.Lives--
			}
			if site.task.Lives <= 0 {
				site.Stop()
			}
		}
		site.results = NewCache(&afterExpire)
		siteMap[key] = site
		go reCaptchaTask(site)
		return nil
	}
}

func StopSite(task *Task) bool {
	lock.Lock()
	defer lock.Unlock()

	key := keyOfTask(task)
	if site, ok := siteMap[key]; ok {
		site.Stop()
		return true
	}
	return false
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

func addSuccessResult(site *Site, captchaID, reCaptchaResponse string) {
	site.ids <- captchaID
	site.results.Set(captchaID, reCaptchaResponse, 100*time.Second)
}

// 获取破解结果
func reCaptchaResult(site *Site, captchaID string) {
	n := 0
	for {
		n++
		log.Printf("查看进度 %v 尝试次数 %v (已有 %v)", captchaID, n, site.results.Keys())

		res, err := client.Get(fmt.Sprintf("http://2captcha.com/res.php?key=%s&action=get&id=%s", API_KEY, captchaID))
		if err != nil {
			log.Println("get:", err)
			if res != nil && res.Body != nil {
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
			addSuccessResult(site, captchaID, reCaptchaResponse)
			return
		}

	wait:
		time.Sleep(2 * time.Second)
	}
}

// 创建破解任务
func reCaptchaTask(site *Site) {
	tasks := make(chan int, site.task.Size)

	newTask := func() {
		defer func() { <-tasks }()

		url := fmt.Sprintf("http://2captcha.com/in.php?key=%s&method=userrecaptcha&googlekey=%s&pageurl=%s", API_KEY, site.task.GoogleKey, site.task.PageURL)
		res, err := client.Post(url, "plain/text", nil)
		if err != nil {
			log.Println(err)
			if res != nil && res.Body != nil {
				res.Body.Close()
			}
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
			// get crack result
			reCaptchaResult(site, captchaID)
		}
	}

	// create new task every task.interval seconds util get the limit of task.Size
	for {
		select {
		case tasks <- 1:
			go newTask()
			time.Sleep(time.Duration(site.task.Interval) * time.Second)
		case <-site.stop:
			log.Println("stop creating new task")
			return
		default:
			time.Sleep(time.Second)
			continue
		}
	}
}

func readBody(res *http.Response) string {
	if res == nil || res.Body == nil {
		return ""
	}
	defer res.Body.Close()

	bodyBytes, err := ioutil.ReadAll(res.Body)
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
