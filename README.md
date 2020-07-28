== Usage

```
lock := lockheed.NewLock("lockname", lockheed.NewKubeLocker(), lockheed.Options{
    Duration: 30 * time.Second,
    RenewInterval: 9 * time.Second,
})
lock.Acquire()
defer lock.Release()
```