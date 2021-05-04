package suckcache

import (
	"context"
	"sync"
	"time"
)

type cacheItem struct {
	time int64
	data interface{}
}

// Cache - кэш с очисткой по таймауту и продлении по обращению. Тип должен быть только ссылочным!!!
type Cache struct {
	timeout  time.Duration
	entities map[string]cacheItem
	mux      sync.Mutex
	add      AddFunc
	update   UpdateFunc
	keys     map[int64][]string
	timer    *time.Ticker
	ctx      context.Context
}

type AddFunc func(key string, param interface{}) (interface{}, error)
type UpdateFunc func(key string, olddata, param interface{}) (interface{}, error)
type CanDeleteFunc func(key string, data interface{}) bool

func NewCache(ctx context.Context, timeout time.Duration, addFunc AddFunc, updateFunc UpdateFunc, canDeleteFunc CanDeleteFunc) *Cache {
	result := &Cache{
		mux:      sync.Mutex{},
		entities: make(map[string]cacheItem),
		keys:     make(map[int64][]string),
		timer:    time.NewTicker(time.Second),
		timeout:  timeout,
		add:      addFunc,
		update:   updateFunc,
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
					newkeys := make([]string, 0, len(keys))
					for _, k := range keys {
						if canDeleteFunc(k, result.entities[k].data) {
							delete(result.entities, k)
						} else {
							newkeys = append(newkeys, k)
						}
					}
					if len(newkeys) == 0 {
						delete(result.keys, t)
					} else {
						for _, k := range newkeys {
							result.tryRenew(k)
						}
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

func (cache *Cache) AddOrUpdate(key string, param interface{}) (interface{}, error) {
	cache.mux.Lock()
	defer cache.mux.Unlock()
	if olddata, ok := cache.tryRenew(key); ok {
		return cache.update(key, olddata, param)
	}
	t := time.Now().Add(cache.timeout).Unix()
	result, err := cache.add(key, param)
	if err != nil {
		return result, err
	}
	cache.entities[key] = cacheItem{data: result, time: t}
	if k, ok := cache.keys[t]; ok {
		cache.keys[t] = append(k, key)
	} else {
		cache.keys[t] = []string{key}
	}
	return result, nil
}

func (cache *Cache) Contains(key string) bool {
	cache.mux.Lock()
	defer cache.mux.Unlock()
	_, ok := cache.entities[key]
	return ok
}

func (cache *Cache) Get(key string) (interface{}, bool) {
	cache.mux.Lock()
	defer cache.mux.Unlock()
	value, ok := cache.entities[key]
	return value.data, ok
}

func (cache *Cache) GetAllKeys() []string {
	cache.mux.Lock()
	defer cache.mux.Unlock()
	result := make([]string, len(cache.entities))
	for k := range cache.entities {
		result = append(result, k)
	}
	return result
}

func (cache *Cache) tryRenew(key string) (interface{}, bool) {
	old, ok := cache.entities[key]
	if !ok {
		return nil, false // TODO WARNING: nil!!!
	}
	t := time.Now().Add(cache.timeout).Unix()
	cache.entities[key] = cacheItem{data: old.data, time: t}
	if k, ok := cache.keys[t]; ok {
		cache.keys[t] = append(k, key)
	} else {
		cache.keys[t] = []string{key}
	}
	a := cache.keys[old.time]
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
		cache.keys[old.time] = b
	} else {
		delete(cache.keys, old.time)
	}
	return old.data, true
}
