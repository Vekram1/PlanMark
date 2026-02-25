# Mixed Fixture

## Source Notes

This fixture blends prose, an ASCII table, and code fences.

| Component | Owner | Status |
|-----------|-------|--------|
| compile   | core  | now    |
| doctor    | core  | next   |

```go
package fixture

func Stable() string { return "deterministic" }
```

- [ ] Build IR from mixed content
  @id fixture.mixed.ir
  @horizon now
  @accept text:verbatim slices preserve table and code fence
  @touches internal/compile/compiler.go
