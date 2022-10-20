# Golang SHell Utilities (gosh)

## What?

A slightly more ergonomic wrapper around `os/exec` which makes things you're used to doing with shells (pipes, parallel processes, input/output redirection, etc) faster.

## Usage

The following demonstrates equivalents to common shell constructs

```go

import (
    sh "github.com/meln5674/gosh/pkg/command"
)

// ...

# $ foo x y z
foo := sh.Command(ctx, "foo", "x", "y", "z")

# $ foo > out
foo.FileOut("out")

# $ foo < in
foo.FileIn("in")

# $ foo 2> err
foo.FileErr("err")

# $ foo <<< "input"
foo.StringIn("input")

# $ BAR=baz QUX=quuz foo
foo.WithParentEnvAnd(map[string]string{"BAR": "baz", "qux": "quux"})

# $ x ; y; z
sh.NewThen(x, y, z)

# $ x && y && z
sh.NewAnd(x, y, z)

# $ x || y || z
sh.NewOr(x, y, z)

# $ x | y | z
sh.NewPipeline(x, y, z)

# $ x & y & z
sh.NewFanout(x, y, z)
```

The Results of `NewCommand`, `NewAnd`, `NewOr`, `NewThen`, `NewPipeline` and `NewFanout` all implement a common interface `Commander`, and can themselves be arguments to `NewAnd`, `NewOr`, `NewThen`, and `NewFanout`. You can use this to define arbitrary complex structures.
