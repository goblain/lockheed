== Usage

```
ctx := context.Background()
lock := lockheed.NewLock("lockname", ctx, lockheed.NewKubeLocker(), lockheed.Options{
    Duration: 30 * time.Second,
    RenewInterval: 9 * time.Second,
})
lock.Acquire()
defer lock.Release()
```