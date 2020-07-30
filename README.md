# Lockheed

Go locking library for kubernetes based locking

## Overview

In one of the projects required locking of some actions in scope of a namespace, hence this library sprung to life.

## Features

* Keeps the state of the lock within a per lock kubernetes ConfigMap object
* Implements interface based abstraction of `Locker` to enable future implementation 
  of other locking backends then kubernetes
* Acquiring and releasing of a mutex lock
* Maintaining a lock with moving time window based on duration and refresh interval
* Dynamic tagging locks
* Listing all locks with filtering based on `Conditions`
* Forcefull takeover of locks based on `Conditions`

## Configuration directives

Any lock object is instantiated in an inactive state. Untill activated by calling lock.Acquire() it can be further configured with following methods, all of which return back (the same) lock object pointer, to allow chaining their invocation like `lock.DirectiveA().DirectiveB()`.

* `.WithDuration(time.Duration)`
  A duration for whitch to establish or renew the lock every time, `0` = infinite
* `.WithRenewInterval(time.Duration)`
  How often to renew the lock, no renewal if not specified
* `.WithContext(context.Context)`
  Use this custom context within the lock
* `.WithTags([]string)`
  Add these tags to the lock state if not already there
* `.WithResetTags()`
  Remove any tags that were not specified withing `.WithTags()` directive
* `.WithForce(Condition)`
  Allow forcefull takeover of a lock if it matches specified condition

## Examples

```
lock := lockheed.NewLock("lockname", lockheed.NewKubeLocker().
    WithDuration(30 * time.Second).
    WithRenewInterval(9 * time.Second)
lock.Acquire()
defer lock.Release()
```

```
lock := lockheed.NewLock("lockname", lockheed.NewKubeLocker().
    WithDuration(30 * time.Second).
    WithRenewInterval(9 * time.Second).
    WithTags([]string{"tag1", "tag2"})
lock.Acquire()
defer lock.Release()
```

```
lock := lockheed.NewLock("lockname", lockheed.NewKubeLocker().
    WithDuration(30 * time.Second).
    WithRenewInterval(9 * time.Second).
    WithResetTags().
    WithForce(lockheed.Condition{
        Operation: lockheed.OperationContains,
        Field:     lockheed.FieldTags,
        Value:     "tag2",
    })
lock.Acquire()
defer lock.Release()
```
