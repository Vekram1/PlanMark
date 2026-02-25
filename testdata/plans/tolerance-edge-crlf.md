# Edge Fixture

## Duplicate

- [ ] Keep odd content
  @id fixture.edge.keep_odd
  @horizon now
  @accept text:coverage accounts for interpreted and opaque lines
  @unknown-meta surprise-value

## Duplicate

This line uses composed form: café
This line uses decomposed form: café

```python
print("open fence starts")

- [ ] Downstream task after broken fence marker
  @id fixture.edge.after_broken_fence
  @horizon next
  @deps fixture.edge.keep_odd
