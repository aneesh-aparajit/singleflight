package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/aneesh-aparajit/singleflight/singleflight"
)

type payload struct {
	key string
	val any
	ttl time.Duration
}

var rc *RedisClient
var sf *singleflight.Singleflight

func main() {
	cfg, err := LoadConfig("./config.yaml")
	if err != nil {
		panic(err)
	}
	rc, err = NewRedisClient(cfg)
	if err != nil {
		panic(err)
	}
	sf = singleflight.NewSingleflight()

	mux := http.NewServeMux()

	mux.HandleFunc("POST /", putValue)
	mux.HandleFunc("GET /", getValue)
	mux.HandleFunc("GET /sf", getValueWithSf)

	if err := http.ListenAndServe(":1337", mux); err != nil {
		panic(err)
	}
}

func putValue(w http.ResponseWriter, r *http.Request) {
	var p payload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"err": "unable to unmarshal data",
		})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()
	rc.Set(ctx, p.key, p.val, p.ttl)
}

func getValue(w http.ResponseWriter, r *http.Request) {
	qp := r.URL.Query()
	key := qp.Get("key")
	if key == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"err": "key not present",
		})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	val, err := rc.Get(ctx, key)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"err": err.Error(),
		})
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"val": val,
	})
}

func getValueWithSf(w http.ResponseWriter, r *http.Request) {
	qp := r.URL.Query()
	key := qp.Get("key")
	if key == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"err": "key not present",
		})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	val, err := sf.Do(ctx, key, func(s string) (any, error) {
		time.Sleep(time.Second * 3)
		rc.rdb.Set(ctx, key, 5, 5*time.Second)
		return 5, nil
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"err": err.Error(),
		})
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"val": val,
	})
}
