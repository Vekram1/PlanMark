# Human Roadmap

## Milestone A

- [ ] Stand up compiler skeleton
  @id fixture.human.compiler_skeleton
  @horizon now
  @accept cmd:go test ./... -run TestCompile
  @why Validate baseline compile path quickly.

## Milestone B

- [ ] Add change detection report
  @id fixture.human.change_report
  @horizon next
  @deps fixture.human.compiler_skeleton
  @why Improve iteration confidence on plan edits.
