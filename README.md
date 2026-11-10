# singleflight

request coalescing (often called "singleflight") means say there are 10k requests arrive for the same key at the same moment, only **one** of them does the expensive work, such as a db query, an upstream api call, or a cache refill. the other 9,999 wait for that one and get its result.

## core idea

keep a map of in-flight work, keyed by request key. the first request for a key creates an entry and does the work. every later request for that key finds the entry and waits on it. when the work is finished, every gets the same result and the entry is removed.

this prevents cache stampede (thundering herd) when a hot key expires.

### usually paired with a cache

```
get(key):
    if cache hit => return
    return singleflight.Do(key, () => {
        v = getFromDb(key)
        cache.set(key, v, ttl)
        return v
    })
```

### failure points

- **leader failure** of the one fetch errors or hangs, all 10k requests fail or hang with it. put a timeout on `fn`. decide whether errors should be shared or whether waiters should retry, perhaps with a fresh leader.
- **waiter cancellation** a waiter whose client disconnects should stop waiting, but it shouldn't cancel the shared work. have waiters `select` on their own context and give the leader a context that isn't tied to any single caller.
- **shared mutable results** all 10k callers get the same object, treat it as read-only or clone it.

### in a distributed environment

- a per-server singleflight still lets each of your N instances hit the DB at once. that's usually fine: N calls instead of 10k.

1. **distributed lock in redis** `SET lock:key <token> NX PX 5000` the winner fetches and fills the cache. losers poll the cache with short backoff or subscribe to a pub/sub "ready" message. combine it with local singleflight so that each pod sends only one request to redis.
2. **consistent-hash routing** route all requests of a specific key to the same pod, which is fine, but if there is a celebrity problem, consider that pod dead.
3. **stale-while-revalidate** keep serving the old cached value, while a single background task refreshes the data, so nobody waits at all. for very hot keys, add cache jitters.

