# eg

A hacked on version of https://github.com/golang/tools/blob/master/cmd/eg/eg.go that switches to using https://godoc.org/golang.org/x/tools/go/packages for module support and a couple of nice-to-haves

## Example usage:

To replace all usages of `errors.Wrap` with the new go errors API.

Template file:
```go
package template

import (
	"fmt"
	"github.com/pkg/errors"
)

func before(err error, s string) error { return errors.Wrap(err, s) }
func after(err error, s string) error  { return fmt.Errorf(s + ": %w", err) }
```

Quick sed script for doing what eg can't (combine string literals AFAICT):
```sed
s!"+": %w!: %w!
```

Usage:

```shell
go run ~/code/eg/eg.go \
  -afteredit 'sed -i.bak -f script.sed {}' \
  -afteredit 'rm -f {}.bak' \
  -afteredit "goimports -w {}" \
  -afteredit "gofmt -w {}" \
  -t template.go \
  -w \
  ./...
```

Example diff:
```diff
        if err := os.Mkdir(full, m.mode); err != nil {
-               return nil, errors.Wrap(err, "create cache")
+               return nil, fmt.Errorf("create cache: %w", err)
        }
```
