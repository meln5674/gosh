# Golang SHell Utilities (GoSh)

## What?

A slightly more ergonomic wrapper around `os/exec` which makes things you're used to doing with shells (pipes, parallel processes, input/output redirection, etc) faster. 
## Usage

To import

```go

import (
    sh "github.com/meln5674/gosh"
)
```

To run a command (process) with common shell-like features, see below
```go
// $ foo x y z
foo := sh.Command("foo", "x", "y", "z")
foo.Run()

// $ foo x y z
// $ wait %1
foo := sh.Command("foo", "x", "y", "z")
if err := foo.Start(); err != nil {
    return err
}
if err := foo.Wait(); err != nil {
    return err
}

// $ foo x y z
// $ kill %1
foo := sh.Command("foo", "x", "y", "z")
if err := foo.Start(); err != nil {
    return err
}
if err := foo.Kill(); err != nil {
    return err
}


// $ foo < in
foo.WithStreams(gosh.FileIn("in")).Run()

// $ foo > out
foo.WithStreams(gosh.FileOut("out")).Run()

// $ foo 2> err
foo.WithStreams(gosh.FileErr("err")).Run()

// $ foo <<< "input"
foo.WithStreams(gosh.StringIn("input")).Run()

// $ {some_go_function} | foo
foo.WithStreams(gosh.FuncIn(some_go_function)).Run()

// $ foo | {some_go_function}
foo.WithStreams(gosh.FuncOut(some_go_function)).Run()

// $ foo 2>&1 >/dev/null/ | {some_go_function}
foo.WithStreams(gosh.FuncErr(some_go_function)).Run()

// $ BAR=baz QUX=quuz foo
foo.WithParentEnvAnd(map[string]string{"BAR": "baz", "qux": "quux"}).Run()

// $ BAR=$(foo)
// For unicode output
var BAR string
foo.WithStreams(gosh.FuncOut(SaveString(&BAR))).Run()
// For binary output
var BAR []byte
foo.WithStreams(gosh.FuncOut(SaveBytes(&BAR))).Run()

// $ BAR+=$(foo)
// For unicode output
BAR := "starting "
foo.WithStreams(gosh.AppendString(&BAR)).Run()
// For binary output
BAR := []byte("starting ")
foo.WithStreams(gosh.AppendBytes(&BAR)).Run()
```

The methods of `Command` follow the builder pattern, and so permit method chaining `which.looks().like().this()`. If an error occurs, all subsequent methods in the chain will do nothing, and that error can be accessed via the `BuilderError` field. If `Run()` or `Start()` is called after an error occurs, it will do nothing and return that error instead.

The `Shell` function will run a script using the current shell

```go
// Same as gosh.Command("${SHELL}", "-c", ...)
script := gosh.Shell(`
while true; do
    echo 'yes'
done
`)
```

By default, `os/exec.Command` will not inherit the parent processes standard in/out/error, and instead use `/dev/null`, opposite of how shells work. To have this behavior, use `WithStreams` with the `Forward*` values.

```go
foo.WithStreams(gosh.ForwardIn)
foo.WithStreams(gosh.ForwardOut)
foo.WithStreams(gosh.ForwardErr)
foo.WithStreams(gosh.ForwardInOut)
foo.WithStreams(gosh.ForwardInErr)
foo.WithStreams(gosh.ForwardOutErr)
foo.WithStreams(gosh.ForwardAll)
```

Commands can be combined in the ways you would expect from a shell (`x`, `y`, and `z` are the outputs of calling `Command`)

```go
// $ x ; y; z
sh.Then(x, y, z).Run()

// $ x && y && z
sh.And(x, y, z).Run()

// $ x || y || z
sh.Or(x, y, z).Run()

// $ x | y | z
sh.Pipeline(x, y, z).Run()

// $ x & y & z &
sh.FanOut(x, y, z).Start()
```

The outputs of these functions have (mostly) the same interface as `Command`, meanning they can be passed back into eachother, with the exception that `Pipeline` cannot accept `FanOut`s.

You can also pass your own types into these functions by implementing the `Commander` interface (for `Then`, `And`, `Or`, and `Fanout`), and `Pipelineable` (For `Pipeline`).

