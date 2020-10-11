package main

import (
	"log"
	"sync"
)

type Task struct {
	GoogleKey string `json:"googleKey"`
	PageURL   string `json:"pageUrl"`
	Size      int    `json:"size"`     // 同时运行的破解任务个数
	Interval  int    `json:"interval"` // 破解间隔时间
	Lives     int    `json:"lives"`    // n 个未使用破解结果，则进入休眠
	NoLimit   bool   `json:"no_limit"` // 不限制过期个数，永久保持热身
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

	if s.idle {
		return
	}

	defer func() {
		log.Println(keyOfTask(s.task), "已停止")
	}()

	s.idle = true
	s.stop <- true
}
