package main

import (
	"container/heap"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"
	"unsafe"
)

var DEBUG = os.Getenv("DEBUG") != ""

// 一个简易的内存缓存系统
// 1. 支持设定过期时间，精度为秒级。
// 2. 支持设定最⼤内存，当内存超出时候做出合理的处理。
// 3. 支持并发安全。

// 支持过期时间和最⼤内存大小的的内存缓存库
type Cacher interface {
	// size 是一个字符串。支持以下参数: 1KB，100KB，1MB，2MB，1GB 等
	SetMaxMemory(size string) bool
	// 设置⼀个缓存项，并且在expire时间之后过期
	Set(key string, val interface{}, expire time.Duration)
	// 获取⼀个值
	Get(key string) (interface{}, bool)
	// 删除⼀个值
	Del(key string) bool
	// 检测⼀个值是否存在
	Exists(key string) bool
	// 清空所有值
	Flush() bool
	// 返回所有的key 多少
	Keys() int64
}

type Cache struct {
	sync.RWMutex
	maxMemory int64
	memory    int64
	mapping   map[string]*Item
	pq        *PriorityQueue
	onExpire  *func(cache *Cache, key string)
}

func NewCache(onExpire *func(cache *Cache, key string)) *Cache {
	cache := &Cache{
		maxMemory: -1,
		memory:    0,
		mapping:   map[string]*Item{},
		pq:        &PriorityQueue{items: []*Item{}},
		onExpire:  onExpire,
	}

	// background gc program to delete unused keys
	go func() {
		for {
			cache.gc()
			time.Sleep(time.Second)
		}
	}()
	return cache
}

func (r *Cache) SetMaxMemory(size string) bool {
	numStr, unit := size[:len(size)-2], size[len(size)-2:]
	num, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		log.Println(err)
		return false
	}

	switch unit {
	case "KB":
		num = num * 1000
	case "MB":
		num = num * 1000 * 1000
	case "GB":
		num = num * 1000 * 1000 * 1000
	default:
		log.Printf("unknown unit %s\n", unit)
		return false
	}

	r.Lock()
	defer r.Unlock()
	r.maxMemory = num

	go r.truncMemory()
	return true
}

func (r *Cache) truncMemory() {
	if r.maxMemory <= 0 {
		return
	}

	r.Lock()
	defer r.Unlock()

	for r.memory > r.maxMemory {
		// avoid popping from blank heap
		if r.pq.Len() == 0 {
			return
		}
		// pop oldest item
		if v := heap.Pop(r.pq).(*Item); v != nil {
			r.del(v, false)
		} else {
			return
		}
	}
}

func (r *Cache) Set(key string, val interface{}, expire time.Duration) {
	r.Lock()
	defer r.Unlock()

	now := time.Now().Unix()
	item := &Item{key, val, now, now + int64(expire/time.Second), -1, int64(unsafe.Sizeof(val))}
	r.mapping[key] = item
	heap.Push(r.pq, item)
	r.memory += item.memory

	go r.truncMemory()
}

func (r *Cache) Get(key string) (interface{}, bool) {
	item := r.getItem(key)
	if item != nil {
		now := time.Now().Unix()
		if item.expire > now {
			return item.value, true
		}
		if r.onExpire != nil {
			go (*r.onExpire)(r, key)
		}
		go r.Del(key)
	}
	return nil, false
}

func (r *Cache) getItem(key string) *Item {
	r.RLock()
	defer r.RUnlock()

	if v, ok := r.mapping[key]; ok {
		return v
	}
	return nil
}

func (r *Cache) Del(key string) bool {
	r.Lock()
	defer r.Unlock()

	if v, ok := r.mapping[key]; ok {
		r.del(v, true)
		return true
	}
	return false
}

func (r *Cache) Exists(key string) bool {
	_, ok := r.Get(key)
	return ok
}

func (r *Cache) Flush() bool {
	r.Lock()
	defer r.Unlock()

	r.mapping = map[string]*Item{}
	r.pq = &PriorityQueue{items: []*Item{}}
	return true
}

func (r *Cache) Keys() (rv int64) {
	r.RLock()
	defer r.RUnlock()

	now := time.Now().Unix()
	for _, v := range r.mapping {
		if v.expire > now {
			rv++
		}
	}
	return
}

func (r *Cache) gc() {
	if len(r.mapping) == 0 {
		return
	}

	if DEBUG {
		log.Println("cache: GCing...")
	}
	r.RLock()
	defer r.RUnlock()

	now := time.Now().Unix()
	for _, v := range r.mapping {
		if v.expire <= now {
			if r.onExpire != nil {
				go (*r.onExpire)(r, v.key)
			}
			go r.Del(v.key)
		}
	}
}

// del without lock
func (r *Cache) del(v *Item, removeHeap bool) {
	delete(r.mapping, v.key)
	if removeHeap {
		heap.Remove(r.pq, v.index)
	}
	r.memory -= v.memory
}

// An Item is something we manage in a priority queue.
type Item struct {
	key      string
	value    interface{}
	priority int64 // seconds elapsed since 1970-Jan-01 UTC
	expire   int64 // seconds elapsed since 1970-Jan-01 UTC
	index    int   // index of element in the heap
	memory   int64 // memory in bytes occupied by value
}

// A PriorityQueue implements heap.Interface and holds Items.
type PriorityQueue struct {
	sync.RWMutex
	items []*Item
}

func (pq *PriorityQueue) Len() int {
	return len(pq.items)
}

func (pq *PriorityQueue) Less(i, j int) bool {
	// min-heap
	return pq.items[i].priority < pq.items[j].priority
}

func (pq PriorityQueue) Swap(i, j int) {
	pq.Lock()
	defer pq.Unlock()

	pq.items[i], pq.items[j] = pq.items[j], pq.items[i]
	pq.items[i].index, pq.items[j].index = i, j
}

func (pq *PriorityQueue) Push(x interface{}) {
	pq.Lock()
	defer pq.Unlock()

	item := x.(*Item)
	item.index = len(pq.items)
	pq.items = append(pq.items, item)
}

func (pq *PriorityQueue) Pop() interface{} {
	pq.Lock()
	defer pq.Unlock()

	n := pq.Len()
	if n == 0 {
		return nil
	}
	item := pq.items[n-1]
	item.index = -1     // mark as poped
	pq.items[n-1] = nil // avoid memory leak
	pq.items = pq.items[0 : n-1]
	return item
}

func test() {
	cache := NewCache(nil)
	cache.Set("a", 1, 3*time.Second)
	cache.Set("b", 1, 3*time.Second)
	log.Println(cache.Get("b"))
	time.Sleep(5 * time.Second)
	log.Println(cache.Get("a"))
	log.Println(cache.Get("b"))
	time.Sleep(1 * time.Second)
	log.Println(cache.mapping, cache.pq.items)

	cache.Set("a", 1, 100*time.Second)
	cache.Set("b", 1, 100*time.Second)
	cache.SetMaxMemory("1KB")
	for i := 1; i < 1200/16; i++ {
		cache.Set(fmt.Sprintf("k%s", i), i, 100*time.Second)
	}
}
