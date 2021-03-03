package suckcache

import (
	"context"
	"sync"
	"time"
)

type dataCacheItem struct {
	time int64
	data []byte
}

// DataCache - кэш с очисткой по таймауту и продлении по обращению
type DataCache struct {
	timeout  time.Duration
	entities map[string]dataCacheItem
	mux      sync.Mutex
	add      func(key string) []byte
	keys     map[int64][]string
	timer    *time.Ticker
	ctx      context.Context
}

func NewDataCache(add func(key string) []byte, timeout time.Duration, ctx context.Context) *DataCache {
	result := &DataCache{
		mux:      sync.Mutex{},
		entities: make(map[string]dataCacheItem),
		keys:     make(map[int64][]string),
		timer:    time.NewTicker(time.Second),
		timeout:  timeout,
		add:      add,
		ctx:      ctx,
	}
	go func() {
		for {
			select {
			case tm := <-result.timer.C:
				t := tm.Unix()
				// log.Println(tm, t, result.keys)
				result.mux.Lock()
				if keys, ok := result.keys[t]; ok {
					delete(result.keys, t)
					for _, k := range keys {
						delete(result.entities, k)
					}
					// log.Println("Delete", keys)
				}
				result.mux.Unlock()
			case <-result.ctx.Done():
				result.timer.Stop()
				return
			}
		}
	}()
	return result
}

func (cache *DataCache) GetOrAdd(key string) []byte {
	t := time.Now().Add(cache.timeout).Unix()
	cache.mux.Lock()
	if data, ok := cache.entities[key]; ok {
		cache.entities[key] = dataCacheItem{data: data.data, time: t}
		if k, ok := cache.keys[t]; ok {
			cache.keys[t] = append(k, key)
		} else {
			cache.keys[t] = []string{key}
		}
		a := cache.keys[data.time]
		index := -1
		for i := 0; i < len(a); i++ {
			if a[i] == key {
				index = i
				break
			}
		}
		if len(a) > 1 {
			b := make([]string, len(a)-1)
			if index > 0 {
				copy(b, a[:index])
			}
			copy(b[index:], a[index+1:])
			cache.keys[data.time] = b
		} else {
			delete(cache.keys, data.time)
		}
		cache.mux.Unlock()
		return data.data
	}
	cache.mux.Unlock()
	data := cache.add(key) // long operation
	cache.mux.Lock()
	cache.entities[key] = dataCacheItem{data: data, time: t}
	if k, ok := cache.keys[t]; ok {
		cache.keys[t] = append(k, key)
	} else {
		cache.keys[t] = []string{key}
	}
	cache.mux.Unlock()
	return data
}
