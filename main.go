package main

import (
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

var (
	API_KEY = getEnvDefault("API_KEY", "")
)

var siteMap = make(map[string]*Site)
var lock = sync.RWMutex{}

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

		v := GetOneCrackResult(task)
		if v == nil {
			c.JSON(http.StatusAccepted, map[string]interface{}{"message": "not ready"})
			return
		}
		c.JSON(http.StatusOK, map[string]interface{}{"g-recaptcha-response": v})
	})

	router.POST("/stopOne", func(c *gin.Context) {
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

		c.JSON(http.StatusOK, map[string]interface{}{"stopped": StopSite(task)})
	})

	router.Run()
}
